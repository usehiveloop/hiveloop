---
name: conversation-assets
description: Use whenever you produce a file in the sandbox that the user should be able to see, download, or share — generated images, videos (e.g. Gemini / Veo output), audio clips, screenshots, CSV/Excel exports, PDFs, zip bundles, anything. Uploads stream straight from the sandbox to public storage and return a stable URL you can paste into your reply. Triggers include "share this video", "give me a link", "show me the chart", "upload the CSV", "send the audio", or any time you've just generated/produced a file the user will want to access outside the sandbox.
---

# Conversation asset uploads

You can store any file produced inside the sandbox to the conversation's asset drive. The upload streams straight to S3, so multi-GB videos work just like small images. The response is JSON containing a `public_url` — paste that URL into your reply so the user can open the asset.

## Two environment variables you'll use

Both are pre-set in the sandbox by the platform — do not try to fetch or rotate them.

| Variable | What it is |
|---|---|
| `HIVELOOP_ASSETS_UPLOAD_URL` | Base URL ending in `…/internal/conversations` |
| `HIVELOOP_CONVERSATION_ID` | UUID of the current conversation |
| `BRIDGE_CONTROL_PLANE_API_KEY` | Bearer token used to authenticate the upload |

The full target URL is:

```
$HIVELOOP_ASSETS_UPLOAD_URL/$HIVELOOP_CONVERSATION_ID/assets/<folder>/<filename>
```

`<folder>` is optional — pick something descriptive (`videos`, `charts`, `exports`) so the assets stay organised when the user views them later. `<filename>` must end in a sensible extension (`.mp4`, `.png`, `.csv`, …) so the content type and the browser preview both work without surprises.

## The one command you need

```bash
curl -fsS -X PUT \
  -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" \
  -H "Content-Type: $(file -b --mime-type ./output.mp4)" \
  --upload-file ./output.mp4 \
  "$HIVELOOP_ASSETS_UPLOAD_URL/$HIVELOOP_CONVERSATION_ID/assets/videos/output.mp4"
```

What each flag does:
- `-X PUT` — the endpoint is PUT.
- `--upload-file` — streams the file body without buffering it in memory. Required for big videos.
- `Content-Type` — set it from the actual file (`file -b --mime-type`) so the browser previews it correctly. If you skip the header, the server falls back to the extension.
- `-fsS` — fail on HTTP errors, stay silent on success, but show errors. Pair with `-o /tmp/upload.json` if you want to capture the response body.

## Reading the response

A successful upload returns `201 Created` with JSON:

```json
{
  "id": "8f6b…",
  "public_url": "https://cdn.example.com/pub/c/<conv>/videos/output.mp4",
  "key": "pub/c/<conv>/videos/output.mp4",
  "path": "videos",
  "filename": "output.mp4",
  "content_type": "video/mp4",
  "bytes": 41284910
}
```

Capture `public_url` and include it in your reply to the user. Example:

```bash
url=$(
  curl -fsS -X PUT \
    -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" \
    -H "Content-Type: video/mp4" \
    --upload-file ./output.mp4 \
    "$HIVELOOP_ASSETS_UPLOAD_URL/$HIVELOOP_CONVERSATION_ID/assets/videos/output.mp4" \
  | jq -r .public_url
)
echo "uploaded to $url"
```

## Deleting an asset

Use `DELETE` against the same URL you uploaded to:

```bash
curl -fsS -X DELETE \
  -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" \
  "$HIVELOOP_ASSETS_UPLOAD_URL/$HIVELOOP_CONVERSATION_ID/assets/videos/output.mp4"
```

Returns `204 No Content` on success, `404` if the asset does not exist in this conversation. Both the S3 object and the database row are removed.

## Moving an asset to a different folder

Only the database `path` label changes — the S3 key (and therefore the public URL) stay put. Use this purely to reorganise how the asset appears in the conversation drive listing.

```bash
curl -fsS -X POST \
  -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"asset":"videos/output.mp4","new_path":"archive/2026"}' \
  "$HIVELOOP_ASSETS_UPLOAD_URL/$HIVELOOP_CONVERSATION_ID/assets/move"
```

`asset` accepts either:

- a relative `<folder>/<filename>` path (e.g. `videos/output.mp4`), or
- the full `public_url` returned at upload time.

`new_path` may be empty (`""`) to move the asset to the drive root. Returns `200 OK` with the updated row.

## Conventions to follow

- **Pick stable filenames.** Re-uploading the same `<folder>/<filename>` overwrites the previous version (drive semantics) — this is intentional for "regenerate the chart" flows. If you want to keep both, vary the filename (`chart-v1.png`, `chart-v2.png`).
- **Group by kind.** `videos/`, `images/`, `audio/`, `charts/`, `exports/` are good defaults. Avoid putting everything at the root.
- **Keep filenames URL-safe.** Stick to `[a-z0-9._-]`. Spaces and most punctuation will work but make the URL ugly when shared.
- **Don't upload secrets.** The URL is public. Anything you put up is world-readable.
- **One file per request.** There's no batch endpoint; loop in shell if you have many files.

## Common mistakes

- **Forgetting the filename in the URL.** The path tail must end in a filename, not a trailing slash. `…/assets/videos/` will return `400 filename is required`.
- **Path traversal.** `..` segments are rejected with `400 invalid path segment`. Just use plain folder names.
- **Wrong content type.** If you upload an `.mp4` with `Content-Type: text/plain`, the browser will refuse to play it. Set the header (or omit it and let the server infer from the extension).
- **`-d @file` instead of `--upload-file`.** `curl -d @big.mp4` reads the whole file into memory before sending. Always use `--upload-file` for binary uploads.
