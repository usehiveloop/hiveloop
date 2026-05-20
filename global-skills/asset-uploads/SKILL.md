---
name: asset-uploads
description: Use whenever you need a URL for a file produced in the sandbox. The platform provides HIVY_DRIVE_UPLOAD_URL and UPLOAD_BEARER. Upload generated images, videos, audio, screenshots, charts, CSV/Excel exports, PDFs, zip bundles, reports, and PR/demo artifacts here, then share the returned file URL.
---

# Asset uploads

Upload files using `HIVY_DRIVE_UPLOAD_URL` and `UPLOAD_BEARER`.

Use this URL exactly as provided. Append only a filename or relative artifact path.

## Environment

Required:

| Variable | Purpose |
|---|---|
| `HIVY_DRIVE_UPLOAD_URL` | Asset upload folder for this sandbox/task |
| `UPLOAD_BEARER` | Bearer token for uploads |

```bash
test -n "$HIVY_DRIVE_UPLOAD_URL" || { echo "HIVY_DRIVE_UPLOAD_URL is not set" >&2; exit 1; }
test -n "$UPLOAD_BEARER" || { echo "upload bearer is not set" >&2; exit 1; }
```

## Upload command

```bash
url=$(
  curl -fsS -X PUT \
    -H "Authorization: Bearer $UPLOAD_BEARER" \
    -H "Content-Type: $(file -b --mime-type ./output.png)" \
    --upload-file ./output.png \
    "$HIVY_DRIVE_UPLOAD_URL/output.png" \
  | jq -r .asset_url
)
printf '%s\n' "$url"
```

For organized artifacts, append a relative path below the assigned folder:

```bash
curl -fsS -X PUT \
  -H "Authorization: Bearer $UPLOAD_BEARER" \
  -H "Content-Type: text/csv" \
  --upload-file ./metrics.csv \
  "$HIVY_DRIVE_UPLOAD_URL/artifacts/metrics.csv"
```

## Response

Successful uploads return `201 Created` with JSON containing the returned URL field:

```json
{
  "id": "...",
  "asset_url": "https://...",
  "key": "pub/e/.../output.png",
  "path": "...",
  "filename": "output.png",
  "content_type": "image/png",
  "bytes": 41284
}
```

Use the returned file URL in replies, PR descriptions, reports, or handoff notes.

## Rules

- Use `HIVY_DRIVE_UPLOAD_URL` exactly as provided.
- Use one PUT request per file.
- Use `--upload-file` for binary and large files.
- Keep filenames URL-safe: lowercase letters, numbers, `-`, `_`, and `.`.
- Do not upload secrets.
