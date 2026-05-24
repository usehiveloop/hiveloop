#!/usr/bin/env python3
"""Tiny webhook receiver for E2E tests.

Listens on HOST:PORT and writes every event from every POST batch as a
single JSON line into OUTFILE (JSONL). Bridge POSTs JSON arrays of
BridgeEvent objects; we flatten them out so the assertion script can
trivially sort by sequence_number and check for gaps.

Usage:
    python3 webhook_receiver.py PORT OUTFILE
"""
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    outfile = None

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length).decode("utf-8") if length else "[]"
        try:
            events = json.loads(body)
            if not isinstance(events, list):
                events = [events]
        except json.JSONDecodeError:
            events = []
        with open(self.outfile, "a") as f:
            for ev in events:
                f.write(json.dumps(ev) + "\n")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

    def log_message(self, *args, **kwargs):
        # Silence default access logs.
        pass


def main():
    if len(sys.argv) != 3:
        print("usage: webhook_receiver.py PORT OUTFILE", file=sys.stderr)
        sys.exit(2)
    port = int(sys.argv[1])
    Handler.outfile = sys.argv[2]
    open(Handler.outfile, "w").close()
    server = ThreadingHTTPServer(("0.0.0.0", port), Handler)
    print(f"webhook receiver listening on :{port} → {Handler.outfile}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
