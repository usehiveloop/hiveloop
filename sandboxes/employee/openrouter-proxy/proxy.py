#!/usr/bin/env python3
"""Transparent OpenRouter proxy that logs every request and response.

The proxy keeps the OpenAI-compatible path shape intact. Point model configs at
http://127.0.0.1:7081/api/v1 and requests will be forwarded to
https://openrouter.ai/api/v1.
"""

from __future__ import annotations

import argparse
import datetime as dt
import http.server
import json
import os
import socketserver
import sys
import time
import uuid
from pathlib import Path
from typing import Iterable
from urllib.parse import urlparse, urlsplit

import requests
from requests import exceptions as requests_exceptions


DEFAULT_TARGET = "https://openrouter.ai/api/v1"
DEFAULT_LISTEN = "127.0.0.1"
DEFAULT_PORT = 7081
DEFAULT_LOG_DIR = Path(__file__).resolve().parent / "logs"
ARIA_PROMPT_PREFIX = "You are Aria,"

HOP_BY_HOP_HEADERS = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailer",
    "transfer-encoding",
    "upgrade",
}

SENSITIVE_HEADERS = {
    "authorization",
    "cookie",
    "set-cookie",
    "x-api-key",
}


def utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat()


def decode_body(body: bytes) -> str:
    return body.decode("utf-8", errors="replace")


def redact_headers(headers: Iterable[tuple[str, str]]) -> dict[str, str]:
    redacted: dict[str, str] = {}
    for key, value in headers:
        redacted[key] = "[redacted]" if key.lower() in SENSITIVE_HEADERS else value
    return redacted


def target_for_request(target_base: str, request_target: str) -> tuple[str, str]:
    base = urlparse(target_base)
    incoming = urlsplit(request_target)

    base_path = base.path.rstrip("/")
    incoming_path = incoming.path or "/"
    suffix = incoming_path
    if base_path and (incoming_path == base_path or incoming_path.startswith(base_path + "/")):
        suffix = incoming_path[len(base_path) :] or "/"
    if not suffix.startswith("/"):
        suffix = "/" + suffix

    target_path = base_path + suffix
    if incoming.query:
        target_path = f"{target_path}?{incoming.query}"

    target_url = f"{base.scheme}://{base.netloc}{target_path}"
    return base.netloc, target_url


def forward_headers(handler: http.server.BaseHTTPRequestHandler, netloc: str, body: bytes) -> dict[str, str]:
    headers: dict[str, str] = {
        "Host": netloc,
        "Accept-Encoding": "identity",
        "Connection": "close",
        "Content-Length": str(len(body)),
    }
    for key, value in handler.headers.items():
        lowered = key.lower()
        if lowered in HOP_BY_HOP_HEADERS or lowered in {"host", "accept-encoding", "content-length"}:
            continue
        headers[key] = value
    return headers


def env_flag(name: str, default: bool) -> bool:
    value = os.environ.get(name)
    if value is None:
        return default
    return value.lower() not in {"0", "false", "no", "off"}


def text_block(text: str, cache_control: bool) -> list[dict[str, object]]:
    block: dict[str, object] = {
        "type": "text",
        "text": text,
    }
    if cache_control:
        block["cache_control"] = {"type": "ephemeral"}
    return [block]


def normalize_chat_completion_body(body: bytes, strategy: str) -> tuple[bytes, dict[str, object]]:
    """Normalize model requests for cache diagnostics.

    OpenRouter/Gemini cache hits depend on a stable leading request shape. The
    bridge currently emits the instruction as a user message and the tool list
    arrives in a nondeterministic order, so this proxy fixes those properties
    before forwarding.
    """
    try:
        payload = json.loads(body)
    except json.JSONDecodeError:
        return body, {"enabled": False, "reason": "body is not JSON"}

    if not isinstance(payload, dict) or "messages" not in payload:
        return body, {"enabled": False, "reason": "not a chat-completion payload"}

    changes: list[str] = []
    messages = payload.get("messages")
    if isinstance(messages, list) and messages:
        first = messages[0]
        if (
            isinstance(first, dict)
            and first.get("role") == "user"
            and isinstance(first.get("content"), str)
            and first["content"].startswith(ARIA_PROMPT_PREFIX)
        ):
            first["role"] = "system"
            first["content"] = text_block(first["content"], cache_control=True)
            changes.append("converted_first_aria_user_message_to_system_cache_block")

        if strategy == "all":
            converted = 0
            for message in messages:
                if not isinstance(message, dict):
                    continue
                content = message.get("content")
                if isinstance(content, str) and content:
                    message["content"] = text_block(content, cache_control=True)
                    converted += 1
                elif isinstance(content, list):
                    for part in content:
                        if isinstance(part, dict) and part.get("type") == "text":
                            part["cache_control"] = {"type": "ephemeral"}
            if converted:
                changes.append(f"converted_all_string_messages_to_cache_blocks:{converted}")
        else:
            # For Gemini, OpenRouter uses the final cache_control breakpoint.
            # Mark the latest cacheable message so subsequent growing turns can
            # reuse the already-seen prefix.
            for message in reversed(messages):
                if not isinstance(message, dict):
                    continue
                content = message.get("content")
                if isinstance(content, str) and content:
                    message["content"] = text_block(content, cache_control=True)
                    changes.append(f"added_cache_control_to_last_{message.get('role', 'unknown')}_message")
                    break

    tools = payload.get("tools")
    if isinstance(tools, list):
        before = json.dumps(tools, separators=(",", ":"), ensure_ascii=False)

        def tool_sort_key(tool: object) -> str:
            if isinstance(tool, dict):
                function = tool.get("function")
                if isinstance(function, dict):
                    return str(function.get("name", ""))
            return json.dumps(tool, sort_keys=True, separators=(",", ":"), ensure_ascii=False)

        tools.sort(key=tool_sort_key)
        after = json.dumps(tools, separators=(",", ":"), ensure_ascii=False)
        if before != after:
            changes.append("sorted_tools_by_function_name")

    normalized = json.dumps(payload, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    return normalized, {
        "enabled": True,
        "changed": normalized != body,
        "changes": changes,
        "strategy": strategy,
        "original_bytes": len(body),
        "upstream_bytes": len(normalized),
    }


class OpenRouterProxyHandler(http.server.BaseHTTPRequestHandler):
    server_version = "openrouter-debug-proxy/1.0"

    def log_message(self, fmt: str, *args: object) -> None:
        sys.stderr.write("[%s] %s\n" % (utc_now(), fmt % args))

    def do_GET(self) -> None:
        self.handle_proxy()

    def do_POST(self) -> None:
        self.handle_proxy()

    def do_PUT(self) -> None:
        self.handle_proxy()

    def do_PATCH(self) -> None:
        self.handle_proxy()

    def do_DELETE(self) -> None:
        self.handle_proxy()

    def do_OPTIONS(self) -> None:
        self.handle_proxy()

    def do_HEAD(self) -> None:
        self.handle_proxy()

    def read_request_body(self) -> bytes:
        length = int(self.headers.get("Content-Length", "0") or "0")
        return self.rfile.read(length) if length else b""

    def handle_proxy(self) -> None:
        request_id = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
        request_id = f"{request_id}-{uuid.uuid4().hex[:8]}"
        started = time.monotonic()
        request_body = self.read_request_body()
        upstream_body = request_body
        normalization: dict[str, object] = {"enabled": False}
        response_body = bytearray()
        response_status: int | None = None
        response_reason: str | None = None
        response_headers: list[tuple[str, str]] = []
        target_url = ""
        error: str | None = None
        upstream_attempts: list[str] = []

        try:
            netloc, target_url = target_for_request(self.server.target_base, self.path)
            if self.server.normalize_cache and self.path.startswith("/api/v1/chat/completions"):
                upstream_body, normalization = normalize_chat_completion_body(
                    request_body,
                    strategy=self.server.cache_strategy,
                )
            headers = forward_headers(self, netloc, upstream_body)
            upstream = self.open_upstream_with_retries(
                target_url=target_url,
                headers=headers,
                body=upstream_body,
                attempts_log=upstream_attempts,
            )
            response_status = upstream.status_code
            response_reason = upstream.reason
            response_headers = list(upstream.headers.items())

            self.send_response(upstream.status_code, upstream.reason)
            for key, value in response_headers:
                if key.lower() in HOP_BY_HOP_HEADERS:
                    continue
                self.send_header(key, value)
            self.end_headers()

            if self.command != "HEAD":
                for chunk in upstream.iter_content(chunk_size=64 * 1024):
                    if not chunk:
                        continue
                    response_body.extend(chunk)
                    self.wfile.write(chunk)
                    self.wfile.flush()
            upstream.close()
        except BrokenPipeError:
            error = "client disconnected while streaming response"
        except Exception as exc:  # noqa: BLE001 - this is a diagnostics proxy.
            error = repr(exc)
            if response_status is None:
                response_status = 502
                response_reason = "Bad Gateway"
                payload = json.dumps({"error": "openrouter proxy upstream failure", "detail": error}).encode()
                response_body.extend(payload)
                self.send_response(502, "Bad Gateway")
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)
        finally:
            self.write_transaction_log(
                request_id=request_id,
                started_at=utc_now(),
                duration_ms=round((time.monotonic() - started) * 1000, 3),
                target_url=target_url,
                request_body=request_body,
                upstream_body=upstream_body,
                normalization=normalization,
                response_status=response_status,
                response_reason=response_reason,
                response_headers=response_headers,
                response_body=bytes(response_body),
                error=error,
                upstream_attempts=upstream_attempts,
            )

    def open_upstream_with_retries(
        self,
        *,
        target_url: str,
        headers: dict[str, str],
        body: bytes,
        attempts_log: list[str],
    ) -> requests.Response:
        last_error: Exception | None = None
        for attempt in range(1, self.server.upstream_max_attempts + 1):
            try:
                response = requests.request(
                    self.command,
                    target_url,
                    data=body,
                    headers=headers,
                    stream=True,
                    timeout=(10, self.server.upstream_timeout_seconds),
                )
                attempts_log.append(f"attempt {attempt}: connected")
                return response
            except (
                requests_exceptions.SSLError,
                requests_exceptions.ConnectionError,
                requests_exceptions.ChunkedEncodingError,
            ) as exc:
                last_error = exc
                attempts_log.append(f"attempt {attempt}: {type(exc).__name__}: {exc}")
                if attempt < self.server.upstream_max_attempts:
                    time.sleep(min(0.25 * attempt, 1.0))
                    continue
                raise
        raise RuntimeError(f"upstream request failed without response: {last_error!r}")

    def write_transaction_log(
        self,
        *,
        request_id: str,
        started_at: str,
        duration_ms: float,
        target_url: str,
        request_body: bytes,
        upstream_body: bytes,
        normalization: dict[str, object],
        response_status: int | None,
        response_reason: str | None,
        response_headers: list[tuple[str, str]],
        response_body: bytes,
        error: str | None,
        upstream_attempts: list[str],
    ) -> None:
        log_dir = self.server.log_dir
        log_dir.mkdir(parents=True, exist_ok=True)
        transaction = {
            "id": request_id,
            "started_at": started_at,
            "duration_ms": duration_ms,
            "request": {
                "method": self.command,
                "path": self.path,
                "target_url": target_url,
                "headers": redact_headers(self.headers.items()),
                "body": decode_body(request_body),
            },
            "upstream_request": {
                "body": decode_body(upstream_body),
                "normalization": normalization,
            },
            "response": {
                "status": response_status,
                "reason": response_reason,
                "headers": redact_headers(response_headers),
                "body": decode_body(response_body),
            },
            "error": error,
            "upstream_attempts": upstream_attempts,
        }
        path = log_dir / f"{request_id}.json"
        tmp_path = path.with_suffix(".json.tmp")
        tmp_path.write_text(json.dumps(transaction, indent=2, ensure_ascii=False), encoding="utf-8")
        os.replace(tmp_path, path)


class ThreadingHTTPServer(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Log-and-forward OpenRouter proxy")
    parser.add_argument("--listen", default=os.environ.get("OPENROUTER_PROXY_LISTEN", DEFAULT_LISTEN))
    parser.add_argument(
        "--port",
        type=int,
        default=int(os.environ.get("OPENROUTER_PROXY_PORT", str(DEFAULT_PORT))),
    )
    parser.add_argument("--target", default=os.environ.get("OPENROUTER_PROXY_TARGET", DEFAULT_TARGET))
    parser.add_argument(
        "--log-dir",
        type=Path,
        default=Path(os.environ.get("OPENROUTER_PROXY_LOG_DIR", str(DEFAULT_LOG_DIR))),
    )
    parser.add_argument(
        "--upstream-timeout-seconds",
        type=float,
        default=float(os.environ.get("OPENROUTER_PROXY_UPSTREAM_TIMEOUT_SECONDS", "300")),
    )
    parser.add_argument(
        "--upstream-max-attempts",
        type=int,
        default=int(os.environ.get("OPENROUTER_PROXY_UPSTREAM_MAX_ATTEMPTS", "4")),
    )
    parser.add_argument(
        "--normalize-cache",
        action=argparse.BooleanOptionalAction,
        default=env_flag("OPENROUTER_PROXY_NORMALIZE_CACHE", True),
        help="normalize chat-completion requests for cache diagnostics",
    )
    parser.add_argument(
        "--cache-strategy",
        choices=["latest", "all"],
        default=os.environ.get("OPENROUTER_PROXY_CACHE_STRATEGY", "latest"),
        help="cache-control placement strategy for normalized requests",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    server = ThreadingHTTPServer((args.listen, args.port), OpenRouterProxyHandler)
    server.target_base = args.target.rstrip("/")
    server.log_dir = args.log_dir
    server.upstream_timeout_seconds = args.upstream_timeout_seconds
    server.upstream_max_attempts = max(1, args.upstream_max_attempts)
    server.normalize_cache = args.normalize_cache
    server.cache_strategy = args.cache_strategy

    print(
        f"openrouter proxy listening on http://{args.listen}:{args.port}; "
        f"forwarding to {server.target_base}; logs in {server.log_dir}; "
        f"normalize_cache={server.normalize_cache}; cache_strategy={server.cache_strategy}",
        flush=True,
    )
    server.serve_forever()


if __name__ == "__main__":
    main()
