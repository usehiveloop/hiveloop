#!/usr/bin/env python3
"""Compare cache metrics for captured proxy transaction logs."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from replay_requests import BREAKDOWN_20


def parse_usage(response_body: str) -> dict:
    usage = None
    for line in response_body.splitlines():
        if not line.startswith("data: "):
            continue
        payload = line[6:]
        if payload == "[DONE]":
            continue
        try:
            event = json.loads(payload)
        except json.JSONDecodeError:
            continue
        if event.get("usage"):
            usage = event["usage"]
    if usage is not None:
        return usage
    try:
        return json.loads(response_body).get("usage") or {}
    except json.JSONDecodeError:
        return {}


def summarize(log_dir: Path, breakdown_20: bool = False) -> dict:
    if breakdown_20:
        files = [log_dir / name for name in BREAKDOWN_20]
    else:
        files = sorted(log_dir.glob("*.json"), key=lambda p: p.stat().st_mtime)
    rows = []
    for path in files:
        transaction = json.loads(path.read_text(encoding="utf-8"))
        usage = parse_usage(transaction["response"]["body"])
        details = usage.get("prompt_tokens_details") or {}
        prompt_tokens = usage.get("prompt_tokens") or 0
        cached_tokens = details.get("cached_tokens") or 0
        cache_write_tokens = details.get("cache_write_tokens") or 0
        rows.append(
            {
                "file": path.name,
                "status": transaction["response"]["status"],
                "error": transaction.get("error"),
                "prompt_tokens": prompt_tokens,
                "cached_tokens": cached_tokens,
                "cache_write_tokens": cache_write_tokens,
                "total_tokens": usage.get("total_tokens") or 0,
                "cost": usage.get("cost") or 0,
                "normalization": (transaction.get("upstream_request") or {}).get("normalization"),
            }
        )

    with_prompt = [row for row in rows if row["prompt_tokens"]]
    prompt_total = sum(row["prompt_tokens"] for row in with_prompt)
    cached_total = sum(row["cached_tokens"] for row in with_prompt)
    write_total = sum(row["cache_write_tokens"] for row in with_prompt)
    cost_total = sum(row["cost"] for row in rows)
    return {
        "log_dir": str(log_dir),
        "request_count": len(rows),
        "requests_with_prompt_tokens": len(with_prompt),
        "http_200_count": sum(1 for row in rows if row["status"] == 200),
        "prompt_tokens": prompt_total,
        "cached_tokens": cached_total,
        "cache_write_tokens": write_total,
        "weighted_cache_hit_ratio": cached_total / prompt_total if prompt_total else 0,
        "cost": cost_total,
        "rows": rows,
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Analyze cache metrics in proxy logs")
    parser.add_argument("--breakdown-20", action="store_true", help="only analyze the fixed 20 source files")
    parser.add_argument("log_dirs", nargs="+", type=Path)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    print(json.dumps([summarize(path, breakdown_20=args.breakdown_20) for path in args.log_dirs], indent=2))


if __name__ == "__main__":
    main()
