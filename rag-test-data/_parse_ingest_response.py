#!/usr/bin/env python3
"""Parse an IngestBatchResponse JSON file and emit shell KEY=VALUE lines."""
import json
import sys

with open(sys.argv[1]) as fh:
    d = json.load(fh)

t = d.get("totals", {})
fails = []
for r in d.get("results", []):
    if r.get("status") == "DOCUMENT_STATUS_FAILED":
        fails.append((r.get("docId", ""), r.get("errorCode", ""), r.get("errorReason", "")))
        if len(fails) >= 3:
            break

print(f"BROWS={int(t.get('rowsWritten', 0))}")
print(f"BBATCH_MS={int(t.get('batchDurationMs', 0))}")
print(f"BCHUNK_MS={int(t.get('chunkDurationMs', 0))}")
print(f"BEMBED_MS={int(t.get('embeddingDurationMs', 0))}")
print(f"BWRITE_MS={int(t.get('writeDurationMs', 0))}")
print(f"BSUCC={int(t.get('docsSucceeded', 0))}")
print(f"BFAIL={int(t.get('docsFailed', 0))}")
print(f"BSKIP={int(t.get('docsSkipped', 0))}")

for idx, (docid, code, reason) in enumerate(fails):
    safe = (reason or "").replace("'", "").replace("\n", " ")[:120]
    print(f"FAIL_SAMPLE_{idx}='{docid}|{code}|{safe}'")

print(f"FAIL_SAMPLE_COUNT={len(fails)}")
