---
name: employee-public-assets-uploads
description: Use whenever you need a public URL for a file that belongs to YOU as an employee — the asset persists across every conversation you have, not just this one. Right tool for things like a profile picture you generated for yourself, a long-lived deliverable a coworker will reference next week, a reusable export, an artifact you'll attach to many PRs, or anything where the asset's identity is "the employee" rather than "this chat thread". Triggers include "upload my avatar", "save this so I can reuse it", "host this for the team", "generate a profile image", "publish my latest report", "I'll need this URL in future conversations". If the file only matters for this conversation, use the conversation-scoped uploader instead.
---

# Employee public asset uploads

You can store any file produced inside the sandbox to your **employee drive** — a public asset bucket scoped to you (the employee), not to a single conversation. The upload streams straight to S3, so multi-GB files work just like small images. The response is JSON containing a `public_url` — paste that URL anywhere the user (or another system) needs to fetch the asset.

## When to use this vs the conversation uploader

| Question | Use this | Use conversation uploader |
|---|---|---|
| Will the asset still matter next week, in a different conversation? | ✅ | ❌ |
| Is it a per-chat artifact (a chart for this thread, a one-shot screenshot)? | ❌ | ✅ |
| Is it part of your identity (avatar, signature image, persona art)? | ✅ | ❌ |
| Will a coworker ask "where did Atlas put that file?" later? | ✅ | ❌ |

When in doubt, ask: *would I want this URL surviving after this conversation ends?* If yes — employee drive. If no — conversation drive.

## The environment variables

Pre-set in the sandbox by the platform — do not try to fetch or rotate them.

| Variable | What it is |
|---|---|
| `HIVELOOP_EMPLOYEE_ASSETS_UPLOAD_URL` | Base URL ending in `…/internal/employees` |
| `HIVELOOP_AGENT_ID` | UUID of you, the employee |
| `BRIDGE_CONTROL_PLANE_API_KEY` | Bearer token used to authenticate the upload |

The full target URL is:

```
$HIVELOOP_EMPLOYEE_ASSETS_UPLOAD_URL/$HIVELOOP_AGENT_ID/assets/<folder>/<filename>
```

`<folder>` is optional — pick something descriptive (`avatars`, `deliverables`, `signatures`) so the assets stay organised. `<filename>` must end in a sensible extension (`.png`, `.csv`, `.pdf`, …) so the content type and the browser preview both work without surprises.

## The one command you need

```bash
curl -fsS -X PUT \
  -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" \
  -H "Content-Type: $(file -b --mime-type ./avatar.png)" \
  --upload-file ./avatar.png \
  "$HIVELOOP_EMPLOYEE_ASSETS_UPLOAD_URL/$HIVELOOP_AGENT_ID/assets/avatars/profile.png"
```

What each flag does:
- `-X PUT` — the endpoint is PUT.
- `--upload-file` — streams the file body without buffering it in memory. Required for big files.
- `Content-Type` — set it from the actual file (`file -b --mime-type`) so the browser previews it correctly. If you skip the header, the server falls back to the extension.
- `-fsS` — fail on HTTP errors, stay silent on success, but show errors. Pair with `-o /tmp/upload.json` if you want to capture the response body.

## Reading the response

A successful upload returns `201 Created` with JSON:

```json
{
  "id": "8f6b…",
  "public_url": "https://cdn.example.com/pub/e/<employee>/avatars/profile.png",
  "key": "pub/e/<employee>/avatars/profile.png",
  "path": "avatars",
  "filename": "profile.png",
  "content_type": "image/png",
  "bytes": 41284
}
```

Capture `public_url` and use it. Example:

```bash
url=$(
  curl -fsS -X PUT \
    -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" \
    -H "Content-Type: image/png" \
    --upload-file ./avatar.png \
    "$HIVELOOP_EMPLOYEE_ASSETS_UPLOAD_URL/$HIVELOOP_AGENT_ID/assets/avatars/profile.png" \
  | jq -r .public_url
)
echo "uploaded to $url"
```

## Deleting an asset

Use `DELETE` against the same URL you uploaded to:

```bash
curl -fsS -X DELETE \
  -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" \
  "$HIVELOOP_EMPLOYEE_ASSETS_UPLOAD_URL/$HIVELOOP_AGENT_ID/assets/avatars/profile.png"
```

Returns `204 No Content` on success, `404` if the asset does not exist on this employee's drive. Both the S3 object and the database row are removed.

## Moving an asset to a different folder

Only the database `path` label changes — the S3 key (and therefore the public URL) stay put. Use this purely to reorganise how the asset appears in your employee drive listing.

```bash
curl -fsS -X POST \
  -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"asset":"avatars/profile.png","new_path":"archive/2026"}' \
  "$HIVELOOP_EMPLOYEE_ASSETS_UPLOAD_URL/$HIVELOOP_AGENT_ID/assets/move"
```

`asset` accepts either:

- a relative `<folder>/<filename>` path (e.g. `avatars/profile.png`), or
- the full `public_url` returned at upload time.

`new_path` may be empty (`""`) to move the asset to the drive root. Returns `200 OK` with the updated row.

## Conventions to follow

- **Pick stable filenames.** Re-uploading the same `<folder>/<filename>` overwrites the previous version (drive semantics) — this is intentional for "regenerate my avatar" flows. If you want to keep both, vary the filename (`avatar-v1.png`, `avatar-v2.png`).
- **Group by purpose.** `avatars/`, `signatures/`, `deliverables/`, `exports/` are good defaults. Avoid putting everything at the root.
- **Keep filenames URL-safe.** Stick to `[a-z0-9._-]`. Spaces and most punctuation will work but make the URL ugly when shared.
- **Don't upload secrets.** The URL is public. Anything you put up is world-readable.
- **One file per request.** There's no batch endpoint; loop in shell if you have many files.

## Common mistakes

- **Uploading per-chat artifacts here.** A screenshot for *this* conversation belongs on the conversation drive — using the employee drive for it pollutes your long-term identity with throwaway files. Re-read the table at the top if unsure.
- **Forgetting the filename in the URL.** The path tail must end in a filename, not a trailing slash. `…/assets/avatars/` will return `400 filename is required`.
- **Path traversal.** `..` segments are rejected with `400 invalid path segment`. Just use plain folder names.
- **Wrong content type.** If you upload an `.png` with `Content-Type: text/plain`, the browser will refuse to render it. Set the header (or omit it and let the server infer from the extension).
- **`-d @file` instead of `--upload-file`.** `curl -d @big.bin` reads the whole file into memory before sending. Always use `--upload-file` for binary uploads.
