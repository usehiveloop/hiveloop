# Public uploads API

**Status:** Plan
**Branch:** `feat/public-uploads-api`

---

## Issues to Address

We have a Railway bucket `public-files` (real S3 name `public-files-eniy12dippoj` on `https://t3.storageapi.dev`) for **publicly readable** assets — avatars, profile pictures, org logos, generic user uploads. Today there's no way to put bytes into it from the product. Users can't change their avatar; org admins can't upload a logo.

This phase adds one endpoint:

```
POST /v1/uploads/sign
```

The endpoint returns a short-lived **presigned PUT URL** plus the **public read URL** the caller will use afterward. Bytes never touch the Go monolith — the client uploads directly to T3 storage. After upload, the client stores the public URL on whatever entity needed it (`users.avatar_url`, `orgs.logo_url`, etc. — those columns are out of scope for this phase; we just hand back the URL).

A second concern this phase resolves: **integration tests need a public-files bucket on the local MinIO.** Currently `make test-services-up` provisions only `hiveloop-rag-test`. We add `public-files-test` to the bootstrap so tests can sign + PUT against it.

---

## Important Notes

### Pattern: presigned PUT, not server-proxy

The bytes never touch our backend. Client `POST /v1/uploads/sign` → backend returns `{ upload_url, required_headers, public_url, expires_at, ... }` → client `PUT upload_url <bytes>`. This matches the existing presigned-URL service the user has for the read flow on a different bucket; same mental model, opposite verb.

We do NOT do server-proxy multipart upload. That'd require streaming bytes through Go (memory + bandwidth + slowness for big files), and we have no current need for server-side scan/transform. Easy to add later if a use case demands it.

### Public read access — bucket policy preferred, ACL fallback

Two ways files become world-readable on T3:
- Bucket policy: `s3:GetObject` for `*` on the bucket. Set once.
- Per-object `x-amz-acl: public-read` baked into each PUT.

T3 advertises S3 compatibility but ACL/bucket-policy support varies. Plan: try the bucket policy first (one `aws s3api put-bucket-policy` call against the prod bucket); if T3 rejects it, fall back to per-object ACL. The signing code supports both via a config flag — production picks one, tests use whichever the local MinIO supports (MinIO supports both).

### Object key strategy

```
avatars/{user_id}/{uuid}.{ext}            user avatar (last write wins for the user_id prefix)
pub/u/{user_id}/{uuid}-{slug}.{ext}       generic user upload
pub/o/{org_id}/{uuid}-{slug}.{ext}        generic org upload
```

UUID in the leaf prevents collisions and keeps old URLs valid even after replacement. `{slug}` is a sanitized version of the client-provided filename — kept for debuggability but not required for correctness.

### Validation lives at sign time, not in the upload itself

The handler enforces, before signing:

| Concern | Enforcement |
|---|---|
| Authenticated caller | `MultiAuth → ResolveOrg` middleware (handler asserts `UserFromContext` is non-nil) |
| `asset_type` is allowed | Enum validated against a hardcoded policy table; unknown kinds → 400 |
| `content_type` matches `asset_type`'s allowlist | e.g. `avatar` only accepts `image/{png,jpeg,webp,gif}` |
| `size_bytes` under per-kind cap | Avatar 5 MiB, generic 25 MiB |
| Org scope (when `asset_type=org_logo`) | `RequireOrgAdmin` for that route |
| Per-user rate limit | 30 sign requests/min; existing rate-limit middleware (`internal/middleware/rate_limit.go`) configured for this endpoint |

The presigned URL bakes in the exact `Content-Type` and `Content-Length` constraint — if the client tries to upload something different, T3 rejects the PUT with a 403/SignatureMismatch. Defense in depth: signed constraint + handler-level pre-check.

### Existing presigner code? Don't know — survey first

The user mentioned "a service to generate presigned url from an s3 bucket" already exists. The implementation agent surveys `internal/storage/`, `internal/handler/`, and the current `cmd/server/serve_routes_v1.go` before writing anything. If there's an existing presigner abstraction, extend it. If not, write a fresh `internal/storage/publicassets.go`. Either way, tightly scoped to this bucket — no shared "all-purpose S3 utility" abstraction.

### File layout

```
internal/storage/
  publicassets.go           ~ 150 LOC  presigner factory, asset-type policy, key builder, public-URL builder
  publicassets_test.go      ~ 180 LOC  signs + PUTs against local MinIO; verifies public GET works

internal/handler/
  uploads.go                ~ 70 LOC   handler struct, response shape
  uploads_sign.go           ~ 130 LOC  POST /v1/uploads/sign — single endpoint
  uploads_sign_test.go      ~ 220 LOC  table-driven: happy paths per asset_type + every rejection branch

cmd/server/serve_routes_v1.go   ~10 line edit  mount the new handler under /v1/uploads
docker-compose.yml              ~3 line edit   minio-setup creates public-files-test alongside hiveloop-rag-test
```

Largest file: `uploads_sign_test.go` ~220 lines, well under the 300-line ceiling.

### Env wiring

Distinct prefix from any existing presign service so configs don't tangle:

```
PUBLIC_ASSETS_S3_ENDPOINT=https://t3.storageapi.dev    # prod; local MinIO http://localhost:9000
PUBLIC_ASSETS_S3_BUCKET=public-files-eniy12dippoj      # prod; local "public-files-test"
PUBLIC_ASSETS_S3_REGION=auto
PUBLIC_ASSETS_ACCESS_KEY_ID=tid_...                    # from `railway bucket credentials`
PUBLIC_ASSETS_SECRET_ACCESS_KEY=tsec_...
PUBLIC_ASSETS_BASE_URL=https://t3.storageapi.dev/public-files-eniy12dippoj  # for constructing public_url
PUBLIC_ASSETS_SIGN_TTL=15m                             # signed URL lifetime
```

`PUBLIC_ASSETS_BASE_URL` is separate from the endpoint so we can swap the read path to a CDN/custom domain later without touching the upload contract.

---

## Implementation Strategy

### Layer A — `internal/storage/publicassets.go`

Surface (kept tight):

```go
type AssetType string
const (
    AssetTypeAvatar    AssetType = "avatar"
    AssetTypeOrgLogo   AssetType = "org_logo"
    AssetTypeGeneric   AssetType = "generic"
)

type SignRequest struct {
    AssetType    AssetType
    UserID       uuid.UUID
    OrgID        *uuid.UUID  // required when AssetType == OrgLogo
    ContentType  string
    SizeBytes    int64
    Filename     string      // optional; sanitized into the key slug
}

type SignedUpload struct {
    UploadURL       string
    UploadMethod    string             // "PUT"
    RequiredHeaders map[string]string  // exact headers the client must send
    Key             string
    PublicURL       string
    ExpiresAt       time.Time
    MaxSizeBytes    int64
}

type Presigner interface {
    Sign(ctx context.Context, req SignRequest) (*SignedUpload, error)
}
```

Policy table (per asset type):
- `avatar` → max 5 MiB; allowed types `image/{png,jpeg,webp,gif}`; key prefix `avatars/{user_id}/`
- `org_logo` → max 5 MiB; same image types; key prefix `pub/o/{org_id}/`
- `generic` → max 25 MiB; broader allowlist (images + PDF + plaintext); key prefix `pub/u/{user_id}/`

Production implementation wraps the AWS SDK v2 presigner against the `PUBLIC_ASSETS_*` env. Test implementation is a real presigner pointed at local MinIO + the `public-files-test` bucket.

### Layer B — `internal/handler/uploads_sign.go`

One handler method:

```go
func (h *UploadsHandler) Sign(w http.ResponseWriter, r *http.Request) {
    user, _ := middleware.UserFromContext(r.Context())
    org, _ := middleware.OrgFromContext(r.Context())
    var req signRequest
    json.Decode(r.Body, &req)
    // validate kind + content-type allowlist + size cap (per asset type)
    out, err := h.presigner.Sign(ctx, storage.SignRequest{...})
    writeJSON(w, 200, out)
}
```

Response shape exactly as the user-facing contract from the strategy doc:

```json
{
  "upload_url": "https://...?X-Amz-Signature=...",
  "upload_method": "PUT",
  "required_headers": { "Content-Type": "image/png", "x-amz-acl": "public-read" },
  "key": "avatars/...",
  "public_url": "https://t3.storageapi.dev/.../avatars/...",
  "expires_at": "2026-04-26T19:00:00Z",
  "max_size_bytes": 5242880
}
```

`org_logo` route is admin-gated — handled at the route mounting layer (`RequireOrgAdmin` for that single path).

### Layer C — Route mount

In `cmd/server/serve_routes_v1.go`:

```go
uploadsHandler := handler.NewUploadsHandler(presigner)
r.Route("/uploads", func(r chi.Router) {
    r.Use(middleware.RequireUser)        // baseline: any authenticated user
    r.Post("/sign", uploadsHandler.Sign) // asset_type-specific authz checked inside the handler
})
```

If existing rate-limit middleware is per-route, attach a `30/minute` limit to this path.

### Layer D — Local MinIO bucket bootstrap

`docker-compose.yml`'s `minio-setup` service today runs:

```yaml
mc alias set local http://minio:9000 minioadmin minioadmin &&
mc mb --ignore-existing local/hiveloop-rag-test &&
echo 'bucket hiveloop-rag-test ready'
```

Extend to:

```yaml
mc alias set local http://minio:9000 minioadmin minioadmin &&
mc mb --ignore-existing local/hiveloop-rag-test &&
mc mb --ignore-existing local/public-files-test &&
mc anonymous set download local/public-files-test &&
echo 'buckets ready'
```

`mc anonymous set download` makes objects in that bucket public-readable by default (matches what we want T3 to do in prod).

### Layer E — Tests

**`publicassets_test.go`** (real local MinIO):

1. `TestSign_AvatarHappyPath` — sign + PUT a 1KB PNG; verify GET on `public_url` returns the bytes (anonymous, no auth).
2. `TestSign_RejectsContentTypeMismatch` — sign for `image/png`, PUT with `Content-Type: text/plain` → MinIO returns 403.
3. `TestSign_RejectsOversizeFile` — sign with `size_bytes=4MB`, PUT 6MB → MinIO returns 400.
4. `TestSign_KeyPrefix_PerAssetType` — verify each asset type produces the right key prefix.
5. `TestSign_ExpiredURL` — sign with `SIGN_TTL=1s`, sleep 2s, PUT → 403.

**`uploads_sign_test.go`** (handler integration):

6. `TestUploadsSign_Avatar_HappyPath` — authenticated user → 200 + valid `SignedUpload`.
7. `TestUploadsSign_OrgLogo_RequiresAdmin` — non-admin → 403; admin → 200 with org_id-scoped key.
8. `TestUploadsSign_RejectsUnknownAssetType` — `asset_type: "weird"` → 400.
9. `TestUploadsSign_RejectsContentTypeOutsideAllowlist` — `asset_type=avatar`, `content_type=application/pdf` → 422.
10. `TestUploadsSign_RejectsOversizeRequest` — `size_bytes=20MB` for `avatar` (cap 5MB) → 422.
11. `TestUploadsSign_RequiresAuthentication` — anonymous → 401 from middleware.

The handler tests use the chi-in-process router pattern established in `internal/handler/in_connections_create_test.go`; the storage tests use the same `connectTestDB` helpers (no DB needed for the storage layer, but the helpers spin up MinIO connectivity).

### Definition of done

- 11 tests passing on real Postgres + real MinIO via `make test-services-up`.
- `scripts/check-go-file-length.sh` clean — every new file under 300 lines.
- `scripts/check-go-comment-density.sh` clean — under 10% comment density.
- `cmd/server` builds; `serve_routes_v1.go` mounts `/v1/uploads/*`.
- A `curl` against the running dev server (auth'd) returns a valid signed URL; the URL works against local MinIO.
- Docs/README mention the four `PUBLIC_ASSETS_*` env vars + how to seed the local MinIO bucket.

---

## Onyx mapping

This is Hiveloop-specific — Onyx doesn't have a public-asset upload endpoint. No upstream port reference.

## Out of scope

- Storing the resulting `public_url` on user/org rows (separate model + migration concern; the upload endpoint hands back the URL, the calling product feature decides what to do with it).
- CDN / custom domain in front of the bucket (one config change in `PUBLIC_ASSETS_BASE_URL` later; doesn't change the upload contract).
- Server-side image resize/transform (no current need; can be added behind the existing endpoint if a caller asks).
- Listing or deleting uploaded objects via API (covered by direct S3 admin access for now).
- Quotas per user/org (per-user rate limit on signs is the only guard in this phase).
