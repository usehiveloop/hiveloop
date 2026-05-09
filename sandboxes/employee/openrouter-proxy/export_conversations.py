#!/usr/bin/env python3
"""Export OpenRouter proxy transaction logs as readable Markdown conversations."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any


DEFAULT_LOG_DIR = Path(__file__).resolve().parent / "logs"
DEFAULT_OUTPUT_DIR = Path(__file__).resolve().parent / "conversations"


def parse_json_object(text: str) -> dict[str, Any]:
    try:
        value = json.loads(text)
    except json.JSONDecodeError:
        return {}
    return value if isinstance(value, dict) else {}


def parse_usage(response_body: str) -> dict[str, Any]:
    usage: dict[str, Any] | None = None
    for event in iter_sse_json(response_body):
        if isinstance(event.get("usage"), dict):
            usage = event["usage"]
    if usage is not None:
        return usage
    return parse_json_object(response_body).get("usage") or {}


def iter_sse_json(response_body: str) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    for line in response_body.splitlines():
        if not line.startswith("data: "):
            continue
        payload = line[6:]
        if not payload or payload == "[DONE]":
            continue
        event = parse_json_object(payload)
        if event:
            events.append(event)
    return events


def parse_response(response_body: str) -> tuple[str, list[dict[str, Any]], dict[str, Any]]:
    text_parts: list[str] = []
    tool_calls: dict[int, dict[str, str]] = {}
    usage: dict[str, Any] = {}

    for event in iter_sse_json(response_body):
        if isinstance(event.get("usage"), dict):
            usage = event["usage"]
        for choice in event.get("choices") or []:
            delta = choice.get("delta") or {}
            if content := delta.get("content"):
                text_parts.append(content)
            for tool_call in delta.get("tool_calls") or []:
                index = int(tool_call.get("index") or 0)
                acc = tool_calls.setdefault(index, {"id": "", "name": "", "arguments": ""})
                if tool_call.get("id"):
                    acc["id"] += tool_call["id"]
                function = tool_call.get("function") or {}
                if function.get("name"):
                    acc["name"] += function["name"]
                if function.get("arguments"):
                    acc["arguments"] += function["arguments"]

    if not text_parts and not tool_calls:
        body = parse_json_object(response_body)
        for choice in body.get("choices") or []:
            message = choice.get("message") or {}
            if content := message.get("content"):
                text_parts.append(content)
            for index, tool_call in enumerate(message.get("tool_calls") or []):
                function = tool_call.get("function") or {}
                tool_calls[index] = {
                    "id": tool_call.get("id") or "",
                    "name": function.get("name") or "",
                    "arguments": function.get("arguments") or "",
                }
        if isinstance(body.get("usage"), dict):
            usage = body["usage"]

    calls = [tool_calls[index] for index in sorted(tool_calls)]
    return "".join(text_parts), calls, usage


def format_json(value: Any) -> str:
    try:
        return json.dumps(value, indent=2, ensure_ascii=False, sort_keys=True)
    except TypeError:
        return str(value)


def compact_json(value: Any, limit: int = 40_000) -> str:
    rendered = format_json(value)
    if len(rendered) <= limit:
        return rendered
    return rendered[:limit] + f"\n... truncated {len(rendered) - limit} chars ..."


def code_block(language: str, content: str) -> str:
    fence = "```"
    while fence in content:
        fence += "`"
    return f"{fence}{language}\n{content}\n{fence}"


def message_text(message: dict[str, Any]) -> str:
    content = message.get("content")
    if isinstance(content, str):
        return content
    if not isinstance(content, list):
        return ""
    parts: list[str] = []
    for part in content:
        if not isinstance(part, dict):
            continue
        if part.get("type") == "text" and isinstance(part.get("text"), str):
            parts.append(part["text"])
        elif part.get("type") == "image_url":
            parts.append(f"[image: {part.get('image_url', {}).get('url', '')[:120]}]")
    return "\n".join(parts)


def cache_controls(message: dict[str, Any]) -> list[dict[str, Any]]:
    controls: list[dict[str, Any]] = []
    content = message.get("content")
    if not isinstance(content, list):
        return controls
    for index, part in enumerate(content):
        if isinstance(part, dict) and "cache_control" in part:
            controls.append({"part": index, "cache_control": part["cache_control"]})
    return controls


def safe_filename(name: str) -> str:
    return re.sub(r"[^A-Za-z0-9_.-]+", "_", name).strip("_")


def render_transaction(path: Path, transaction: dict[str, Any]) -> str:
    request = transaction.get("request") or {}
    response = transaction.get("response") or {}
    request_body = parse_json_object(request.get("body") or "")
    response_text, response_tool_calls, response_usage = parse_response(response.get("body") or "")
    usage = response_usage or parse_usage(response.get("body") or "")
    prompt_details = usage.get("prompt_tokens_details") or {}
    messages = request_body.get("messages") or []
    tools = request_body.get("tools") or []

    lines: list[str] = []
    lines.append(f"# {path.stem}")
    lines.append("")
    lines.append("## Summary")
    lines.append("")
    lines.append(f"- Started: `{transaction.get('started_at')}`")
    lines.append(f"- Duration: `{transaction.get('duration_ms')} ms`")
    lines.append(f"- Target: `{request.get('method')} {request.get('target_url')}`")
    lines.append(f"- Status: `{response.get('status')} {response.get('reason')}`")
    lines.append(f"- Model: `{request_body.get('model')}`")
    lines.append(f"- Messages: `{len(messages)}`")
    lines.append(f"- Tools: `{len(tools)}`")
    if usage:
        prompt_tokens = usage.get("prompt_tokens") or 0
        cached_tokens = prompt_details.get("cached_tokens") or 0
        hit_ratio = cached_tokens / prompt_tokens if prompt_tokens else 0
        lines.append(f"- Prompt tokens: `{prompt_tokens}`")
        lines.append(f"- Cached tokens: `{cached_tokens}`")
        lines.append(f"- Cache hit ratio: `{hit_ratio:.2%}`")
        lines.append(f"- Completion tokens: `{usage.get('completion_tokens') or 0}`")
        lines.append(f"- Total tokens: `{usage.get('total_tokens') or 0}`")
        lines.append(f"- Cost: `${usage.get('cost') or 0}`")
    if transaction.get("error"):
        lines.append(f"- Proxy error: `{transaction['error']}`")

    lines.append("")
    lines.append("## Request Messages")
    for index, message in enumerate(messages):
        role = message.get("role")
        text = message_text(message)
        controls = cache_controls(message)
        lines.append("")
        lines.append(f"### Message {index}: `{role}`")
        lines.append("")
        lines.append(f"- Text chars: `{len(text)}`")
        if message.get("tool_call_id"):
            lines.append(f"- Tool call id: `{message.get('tool_call_id')}`")
        if controls:
            lines.append(f"- Cache controls: `{format_json(controls)}`")
        if message.get("tool_calls"):
            lines.append("")
            lines.append("Tool calls:")
            lines.append(code_block("json", compact_json(message["tool_calls"])))
        if text:
            lines.append("")
            lines.append(code_block("text", text))

    lines.append("")
    lines.append("## Request Tools")
    if tools:
        lines.append("")
        lines.append("| # | Name | Description chars | Schema chars |")
        lines.append("|---:|---|---:|---:|")
        for index, tool in enumerate(tools):
            function = tool.get("function") or {}
            description = function.get("description") or ""
            parameters = function.get("parameters") or {}
            lines.append(
                f"| {index} | `{function.get('name')}` | {len(description)} | {len(format_json(parameters))} |"
            )
        lines.append("")
        lines.append("<details>")
        lines.append("<summary>Full tool schemas</summary>")
        lines.append("")
        lines.append(code_block("json", compact_json(tools)))
        lines.append("")
        lines.append("</details>")
    else:
        lines.append("")
        lines.append("_No tools sent._")

    lines.append("")
    lines.append("## Response")
    if response_text:
        lines.append("")
        lines.append("### Text")
        lines.append("")
        lines.append(code_block("text", response_text))
    if response_tool_calls:
        lines.append("")
        lines.append("### Tool Calls")
        lines.append("")
        lines.append(code_block("json", compact_json(response_tool_calls)))
    if usage:
        lines.append("")
        lines.append("### Usage")
        lines.append("")
        lines.append(code_block("json", format_json(usage)))

    lines.append("")
    lines.append("## Raw Metadata")
    lines.append("")
    lines.append("<details>")
    lines.append("<summary>Headers and proxy metadata</summary>")
    lines.append("")
    metadata = {
        "id": transaction.get("id"),
        "started_at": transaction.get("started_at"),
        "duration_ms": transaction.get("duration_ms"),
        "request": {
            "method": request.get("method"),
            "path": request.get("path"),
            "target_url": request.get("target_url"),
            "headers": request.get("headers"),
        },
        "response": {
            "status": response.get("status"),
            "reason": response.get("reason"),
            "headers": response.get("headers"),
        },
        "upstream_attempts": transaction.get("upstream_attempts"),
        "error": transaction.get("error"),
    }
    lines.append(code_block("json", format_json(metadata)))
    lines.append("")
    lines.append("</details>")
    lines.append("")
    return "\n".join(lines)


def export(log_dir: Path, output_dir: Path, overwrite: bool) -> None:
    output_dir.mkdir(parents=True, exist_ok=True)
    count = 0
    for path in sorted(log_dir.glob("*.json")):
        transaction = parse_json_object(path.read_text(encoding="utf-8"))
        if not transaction:
            continue
        output_path = output_dir / f"{safe_filename(path.stem)}.md"
        if output_path.exists() and not overwrite:
            continue
        output_path.write_text(render_transaction(path, transaction), encoding="utf-8")
        count += 1
    print(f"Exported {count} Markdown files to {output_dir}")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Export OpenRouter proxy logs into readable Markdown files."
    )
    parser.add_argument("--log-dir", type=Path, default=DEFAULT_LOG_DIR)
    parser.add_argument("--output-dir", type=Path, default=DEFAULT_OUTPUT_DIR)
    parser.add_argument("--overwrite", action="store_true")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    export(args.log_dir, args.output_dir, args.overwrite)


if __name__ == "__main__":
    main()
