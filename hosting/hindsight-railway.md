# Hindsight Self-Hosted on Railway

Self-hosted deployment of [Hindsight](https://github.com/vectorize-io/hindsight) — a biomimetic memory system for AI agents — running on Railway within the HiveLoop production project.

## Architecture

```
+--------------------------------------------------------+
|  Railway Project (hiveloop.dev / production)           |
|                                                        |
|  +-------------------------+   +---------------------+ |
|  |  hindsight (slim)       |   |  Railway Postgres   | |
|  |  :8888 API (private)    |-->|  18.3 + pgvector    | |
|  |  :9999 CP  (public*)    |   |  0.8.2 + pg_trgm    | |
|  +------------+------------+   +---------------------+ |
|               ^                                        |
|               | private network                        |
|  +------------+------------+                           |
|  |  hiveloop (Go backend)  |                           |
|  +-------------------------+                           |
+--------------------------------------------------------+
         |              |              |
         v              v              v
    OpenRouter      OpenAI API     ZeroEntropy
  gemini-2.5-flash  embeddings      zerank-2

* CP public domain will be disabled in production.
  Use `railway port-forward` for local access.
```

## Services

| Service | Image | Ports |
|---------|-------|-------|
| **hindsight** | `ghcr.io/vectorize-io/hindsight:latest-slim` | 8888 (API), 9999 (Control Plane) |
| **Postgres** | Railway managed PostgreSQL 18.3 | 5432 (private), 37562 (public TCP proxy) |

The **slim** image (~500MB) does not bundle local ML models. It requires external providers for embeddings and reranking.

## Model Stack

| Component | Provider | Model | Purpose | Cost |
|-----------|----------|-------|---------|------|
| **LLM** | OpenRouter (OpenAI-compatible) | `google/gemini-2.5-flash` | Fact extraction, entity resolution, consolidation | ~$0.15/1M input, $0.60/1M output |
| **Embeddings** | OpenAI | `text-embedding-3-small` (1536 dims) | Semantic vector search | $0.02/1M tokens |
| **Reranker** | ZeroEntropy | `zerank-2` | State-of-the-art result reranking | Per ZeroEntropy pricing |

### Why these models (from Hindsight docs)

- **LLM**: Hindsight docs state "Hindsight doesn't need a smart model" — fact extraction is structured, so fast/cheap models work well. Gemini 2.5 Flash is a tested model with 65K+ output token support (required by Hindsight).
- **Embeddings**: `text-embedding-3-small` is labeled "cost-effective" in the docs. 1536 dimensions is sufficient — the reranker handles precision.
- **Reranker**: `zerank-2` is labeled "Production, state-of-the-art accuracy" in the docs, the highest tier available.

## Networking

| Port | Access | Used by |
|------|--------|---------|
| **8888** (API) | Private only (`hindsight.railway.internal:8888`) | HiveLoop backend, CP (localhost) |
| **9999** (Control Plane) | Public domain (temporary) | Browser — memory bank management UI |

The API is never exposed publicly. Security is provided by Railway's private networking.

### Endpoints

- **API (private)**: `http://hindsight.railway.internal:8888`
- **Control Plane (public)**: `https://hindsight-production-c03a.up.railway.app`
- **Postgres (public TCP proxy)**: `interchange.proxy.rlwy.net:37562`

## Environment Variables

### Database

| Variable | Value | Notes |
|----------|-------|-------|
| `HINDSIGHT_API_DATABASE_URL` | `${{Postgres.DATABASE_URL}}` | Railway reference variable, resolves to internal Postgres URL |
| `HINDSIGHT_API_DATABASE_SCHEMA` | `hindsight` | Dedicated schema, auto-created by migrations |
| `HINDSIGHT_API_RUN_MIGRATIONS_ON_STARTUP` | `true` | Migrations run automatically on deploy |

### LLM (OpenRouter)

| Variable | Value |
|----------|-------|
| `HINDSIGHT_API_LLM_PROVIDER` | `openai` |
| `HINDSIGHT_API_LLM_BASE_URL` | `https://openrouter.ai/api/v1` |
| `HINDSIGHT_API_LLM_API_KEY` | `<OPENROUTER_API_KEY>` |
| `HINDSIGHT_API_LLM_MODEL` | `google/gemini-2.5-flash` |

### Embeddings (OpenAI)

| Variable | Value |
|----------|-------|
| `HINDSIGHT_API_EMBEDDINGS_PROVIDER` | `openai` |
| `HINDSIGHT_API_EMBEDDINGS_OPENAI_API_KEY` | `<OPENAPI_KEY>` |
| `HINDSIGHT_API_EMBEDDINGS_OPENAI_MODEL` | `text-embedding-3-small` |

### Reranker (ZeroEntropy)

| Variable | Value |
|----------|-------|
| `HINDSIGHT_API_RERANKER_PROVIDER` | `zeroentropy` |
| `HINDSIGHT_API_RERANKER_ZEROENTROPY_API_KEY` | `<ZERANK_API_KEY>` |
| `HINDSIGHT_API_RERANKER_ZEROENTROPY_MODEL` | `zerank-2` |

### Server

| Variable | Value | Notes |
|----------|-------|-------|
| `HINDSIGHT_API_HOST` | `0.0.0.0` | Bind to all interfaces |
| `HINDSIGHT_API_PORT` | `8888` | API port |
| `HINDSIGHT_API_WORKERS` | `1` | Single worker (2 causes crash loop in combined image) |
| `HINDSIGHT_API_WORKER_ENABLED` | `true` | Internal background worker for consolidation |
| `HINDSIGHT_API_LOG_LEVEL` | `info` | |
| `HINDSIGHT_API_LOG_FORMAT` | `json` | Structured logging for Railway |

### Variables NOT to set

| Variable | Why |
|----------|-----|
| `PORT` | Controls the CP port in the combined image. Setting it conflicts with the API. Let the image manage ports internally. |
| `HINDSIGHT_API_TENANT_EXTENSION` | Enables API key auth, but the CP has no mechanism to pass auth headers when proxying to the API. Results in "Failed to fetch banks from API" errors. |
| `HINDSIGHT_API_TENANT_API_KEY` | Same as above — do not enable tenant auth with the combined image. |

## PostgreSQL Extensions

Two extensions must exist in the **public** schema:

```sql
CREATE EXTENSION IF NOT EXISTS vector;   -- pgvector for semantic search
CREATE EXTENSION IF NOT EXISTS pg_trgm;  -- trigram matching for entity lookup
```

If `pg_trgm` ends up in the `hindsight` schema (migrations may create it there), move it:

```sql
ALTER EXTENSION pg_trgm SET SCHEMA public;
```

Verify:

```sql
SELECT extname, extnamespace::regnamespace
FROM pg_extension
WHERE extname IN ('vector', 'pg_trgm');
```

Expected output:

```
 extname | extnamespace
---------+--------------
 vector  | public
 pg_trgm | public
```

## HiveLoop Integration

HiveLoop connects to the Hindsight API over Railway's private network:

```
HINDSIGHT_API_URL=http://hindsight.railway.internal:8888
```

No API key is needed — the API is not authenticated (private networking provides security).

### Key API Endpoints

| Operation | Method | Path |
|-----------|--------|------|
| List banks | GET | `/v1/default/banks` |
| Retain (store memories) | POST | `/v1/default/banks/{bank_id}/memories` |
| Recall (search memories) | POST | `/v1/default/banks/{bank_id}/memories/recall` |
| Reflect (reason over memories) | POST | `/v1/default/banks/{bank_id}/reflect` |
| Bank config | GET/PATCH | `/v1/default/banks/{bank_id}/config` |
| Health check | GET | `/health` |
| OpenAPI spec | GET | `/openapi.json` |
| Swagger docs | GET | `/docs` |

## Deployment

### Initial Setup

```bash
# 1. Link to the Railway project
railway link --project <hiveloop-project-id>

# 2. Provision Postgres
railway add --database postgres

# 3. Enable extensions
railway service link <postgres-service-id>
railway connect postgres
# In psql:
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
\q

# 4. Create Hindsight service
railway add --service hindsight --image ghcr.io/vectorize-io/hindsight:latest-slim

# 5. Link and set variables
railway service link hindsight
railway variable set \
  HINDSIGHT_API_DATABASE_URL='${{Postgres.DATABASE_URL}}' \
  HINDSIGHT_API_DATABASE_SCHEMA=hindsight \
  HINDSIGHT_API_RUN_MIGRATIONS_ON_STARTUP=true \
  HINDSIGHT_API_LLM_PROVIDER=openai \
  HINDSIGHT_API_LLM_BASE_URL=https://openrouter.ai/api/v1 \
  HINDSIGHT_API_LLM_API_KEY=<openrouter-key> \
  HINDSIGHT_API_LLM_MODEL=google/gemini-2.5-flash \
  HINDSIGHT_API_EMBEDDINGS_PROVIDER=openai \
  HINDSIGHT_API_EMBEDDINGS_OPENAI_API_KEY=<openai-key> \
  HINDSIGHT_API_EMBEDDINGS_OPENAI_MODEL=text-embedding-3-small \
  HINDSIGHT_API_RERANKER_PROVIDER=zeroentropy \
  HINDSIGHT_API_RERANKER_ZEROENTROPY_API_KEY=<zerank-key> \
  HINDSIGHT_API_RERANKER_ZEROENTROPY_MODEL=zerank-2 \
  HINDSIGHT_API_HOST=0.0.0.0 \
  HINDSIGHT_API_PORT=8888 \
  HINDSIGHT_API_WORKERS=1 \
  HINDSIGHT_API_WORKER_ENABLED=true \
  HINDSIGHT_API_LOG_LEVEL=info \
  HINDSIGHT_API_LOG_FORMAT=json

# 6. Generate public domain for Control Plane
railway domain --port 9999 --service hindsight
```

### Redeployment

```bash
railway service link hindsight
railway redeploy --service hindsight --yes
```

### Database Reset (full clean slate)

```bash
# Connect to Postgres
psql "postgresql://postgres:<password>@interchange.proxy.rlwy.net:<port>/railway"

# Drop and recreate
DROP SCHEMA IF EXISTS hindsight CASCADE;

# Ensure extensions are in public schema
CREATE EXTENSION IF NOT EXISTS vector;
ALTER EXTENSION pg_trgm SET SCHEMA public;
\q

# Redeploy to run fresh migrations
railway service link hindsight
railway redeploy --service hindsight --yes
```

## Troubleshooting

### "Child process died" crash loop

**Symptom:** Logs show repeated `Waiting for child process` / `Child process died` messages.

**Cause:** `PORT` env var is set, causing the uvicorn API workers to bind to the same port as the Control Plane (9999).

**Fix:**
```bash
railway variable delete PORT
railway redeploy --service hindsight --yes
```

### "Failed to fetch banks from API" in Control Plane

**Symptom:** CP dashboard loads but shows "Failed to fetch banks from API error".

**Cause:** `HINDSIGHT_API_TENANT_EXTENSION` is enabled. The CP proxy cannot pass auth headers to the API.

**Fix:**
```bash
railway variable delete HINDSIGHT_API_TENANT_EXTENSION
railway variable delete HINDSIGHT_API_TENANT_API_KEY
railway redeploy --service hindsight --yes
```

### "operator does not exist: text % text"

**Symptom:** Retain calls fail with this PostgreSQL error.

**Cause:** `pg_trgm` extension is in the `hindsight` schema instead of `public`.

**Fix:**
```sql
ALTER EXTENSION pg_trgm SET SCHEMA public;
```

### "relation hindsight.async_operations does not exist"

**Symptom:** Worker logs show repeated warnings about missing tables.

**Cause:** The `hindsight` schema was dropped but the service wasn't redeployed, so migrations didn't re-run.

**Fix:**
```bash
railway redeploy --service hindsight --yes
```

### "insufficient_quota" from OpenAI embeddings

**Symptom:** Retain fails with 429 error from OpenAI.

**Cause:** The OpenAI API key has no credits.

**Fix:** Add billing to the OpenAI account, then update the key:
```bash
railway variable set HINDSIGHT_API_EMBEDDINGS_OPENAI_API_KEY=<new-key>
```

### Workers > 1 causes instability

**Symptom:** Setting `HINDSIGHT_API_WORKERS=2` causes child processes to die repeatedly.

**Cause:** Multiple uvicorn workers in the combined image compete for resources or ports.

**Fix:** Keep `HINDSIGHT_API_WORKERS=1`. For higher throughput, deploy the API and CP as separate Railway services using `ghcr.io/vectorize-io/hindsight-api:latest-slim` and `ghcr.io/vectorize-io/hindsight-control-plane:latest`.

## Smoke Test

### From inside the container (via SSH)

```bash
# Retain
railway ssh -s hindsight -- 'curl -s -X POST http://localhost:8888/v1/default/banks/test-bank/memories \
  -H "Content-Type: application/json" \
  -d "{\"items\": [{\"content\": \"Test fact about the system.\", \"context\": \"Testing\"}]}"'

# Recall
railway ssh -s hindsight -- 'curl -s -X POST http://localhost:8888/v1/default/banks/test-bank/memories/recall \
  -H "Content-Type: application/json" \
  -d "{\"query\": \"What do we know about the system?\"}"'

# Health
railway ssh -s hindsight -- curl -s http://localhost:8888/health
```

### From Control Plane (public)

```bash
# List banks (via CP proxy)
curl -s https://hindsight-production-c03a.up.railway.app/api/banks
```

## Monitoring

- **Logs:** `railway logs --service hindsight`
- **Prometheus metrics:** `http://localhost:8888/metrics` (via SSH)
- **Worker stats:** Look for `[WORKER_STATS]` lines in logs — shows slot utilization and pending tasks
- **Railway dashboard:** CPU, memory, network metrics for the hindsight service

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| **Slim image** over full | 500MB vs 9GB — faster deploys, lower cost on Railway. External providers handle embeddings/reranking. |
| **No tenant auth** | Combined image CP can't proxy auth headers. Private networking provides security instead. |
| **Single worker** | Multiple workers crash in the combined image. Scale by splitting into separate API + CP services. |
| **`pg_trgm` in public schema** | Hindsight migrations may create it in the `hindsight` schema, but queries expect it in `public`. |
| **OpenRouter for LLM** | Cost proxy layer — route to cheapest provider for structured extraction. Gemini 2.5 Flash is a tested model. |
| **ZeroEntropy for reranking** | Docs label it "state-of-the-art accuracy" — premium pick for best memory quality. |

## Railway Service IDs

| Resource | ID |
|----------|----|
| **Hindsight service** | `ce28a2cb-1793-4dcb-a70e-7c1b00060cb9` |
| **Postgres service** | `1b93d8f7-95d7-4c5a-a231-1dacda37734d` |
| **Railway project** | `55776e03-e6c2-4a9b-828b-4e759495aa70` |
| **Environment** | `production` (`3c177170-0fb2-4dcb-a034-12676bb242c6`) |

## References

- [Hindsight Configuration Docs](https://docs.hindsight.vectorize.io/developer/configuration)
- [Hindsight Models & Benchmarks](https://benchmarks.hindsight.vectorize.io/)
- [Hindsight API Reference](https://docs.hindsight.vectorize.io/developer/api/quickstart)
- [Railway Private Networking](https://docs.railway.com/reference/private-networking)
