#!/usr/bin/env python3
"""Generate an agent config whose OpenAI-compatible models use the local proxy."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
from typing import Any


DEFAULT_PROXY_BASE_URL = "http://127.0.0.1:7081/api/v1"


def rewrite_model_base_urls(value: Any, proxy_base_url: str) -> Any:
    if isinstance(value, dict):
        rewritten = {
            key: rewrite_model_base_urls(child, proxy_base_url)
            for key, child in value.items()
        }
        if rewritten.get("provider") == "openai_compatible" and "base_url" in rewritten:
            rewritten["base_url"] = proxy_base_url
        return rewritten
    if isinstance(value, list):
        return [rewrite_model_base_urls(item, proxy_base_url) for item in value]
    return value


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Write config JSON that routes model calls through the proxy")
    parser.add_argument("config", type=Path, help="source agent config JSON")
    parser.add_argument(
        "--proxy-base-url",
        default=os.environ.get("OPENROUTER_PROXY_BASE_URL", DEFAULT_PROXY_BASE_URL),
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    config = json.loads(args.config.read_text(encoding="utf-8"))
    rewritten = rewrite_model_base_urls(config, args.proxy_base_url.rstrip("/"))
    print(json.dumps(rewritten, indent=2))


if __name__ == "__main__":
    main()
