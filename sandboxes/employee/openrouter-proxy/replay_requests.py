#!/usr/bin/env python3
"""Replay captured proxy request bodies through a proxy endpoint."""

from __future__ import annotations

import argparse
import json
import os
import time
from pathlib import Path

import requests


BREAKDOWN_20 = [
    "20260509T065946.147792Z-6606af4f.json",
    "20260509T070012.379865Z-84368ed7.json",
    "20260509T070014.390082Z-282aade3.json",
    "20260509T070017.984405Z-e4d658e6.json",
    "20260509T070020.389796Z-b29041bd.json",
    "20260509T070048.289388Z-2465f26c.json",
    "20260509T070049.968179Z-daf8936c.json",
    "20260509T070054.304430Z-2623e6d7.json",
    "20260509T070056.818686Z-53c7ce88.json",
    "20260509T070116.373000Z-ad680595.json",
    "20260509T070118.384847Z-831fdf81.json",
    "20260509T070126.330260Z-4842a2ec.json",
    "20260509T070140.135154Z-c1bafa1c.json",
    "20260509T070141.826420Z-c208b7ca.json",
    "20260509T070145.301526Z-87cadf05.json",
    "20260509T070214.431817Z-c39766cc.json",
    "20260509T070236.029705Z-90423216.json",
    "20260509T070238.710453Z-c9ff35c0.json",
    "20260509T070242.200544Z-3b08802e.json",
    "20260509T070245.168546Z-cb4471f1.json",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Replay the 20 analyzed agent-flow requests")
    parser.add_argument("--source-log-dir", type=Path, default=Path("openrouter-proxy/logs"))
    parser.add_argument("--proxy-url", default="http://127.0.0.1:7081/api/v1/chat/completions")
    parser.add_argument("--delay-seconds", type=float, default=0.25)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    api_key = os.environ["OPENROUTER_API_KEY"]
    for index, filename in enumerate(BREAKDOWN_20, start=1):
        source = args.source_log_dir / filename
        transaction = json.loads(source.read_text(encoding="utf-8"))
        body = transaction["request"]["body"]
        response = requests.post(
            args.proxy_url,
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
            },
            data=body,
            stream=True,
            timeout=(10, 240),
        )
        chunks: list[bytes] = []
        for chunk in response.iter_content(chunk_size=64 * 1024):
            if chunk:
                chunks.append(chunk)
        text = b"".join(chunks).decode("utf-8", errors="replace")
        print(
            f"{index:02d}/{len(BREAKDOWN_20)} {filename} "
            f"status={response.status_code} done={'[DONE]' in text} bytes={len(text.encode())}",
            flush=True,
        )
        time.sleep(args.delay_seconds)


if __name__ == "__main__":
    main()
