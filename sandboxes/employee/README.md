# employee-bridge

Rust runtime for AI agents that act as real-world employees in Slack (and later Discord, Teams, WhatsApp). One sandbox = one workspace = one bot. Built on a provider-agnostic Rig-based agent runtime. Configured live via a control-plane HTTP API.

## Layout

```
crates/
  domain/           # shared types: AgentDefinition, SessionId, InboundEvent, ConfigStore
  gateway/          # ChannelGateway trait + Slack adapter
  agent/            # AgentRunner trait, Rig-based agent runtime
  storage/          # SQLite repos + migrations
  tools/            # bash, read_file, write_file, ...
  mcp/              # MCP server registry
  skills/           # skill prompt composition
  webhooks/         # outbound webhook outbox
  api/              # axum control-plane HTTP
  employee-bridge/  # binary
```

## Phase status

- **Phase 0 — foundations:** trait contracts and shared types (current)
- Phase 1 — five parallel tracks (Slack, agent, storage, api, webhooks)
- Phase 2 — integration; real Slack ping → reply test
- Phase 3 — tools, MCP, skills, subagents, Slack richness
- Phase 4 — observability and hardening

## Sentry

Sentry is disabled unless `SENTRY_DSN` or `SENTRY_SPOTLIGHT=true` is set.

Useful environment variables:

- `SENTRY_DSN`: production Sentry DSN.
- `SENTRY_ENVIRONMENT`: defaults to `APP_ENV`, `RUST_ENV`, then `development`.
- `SENTRY_RELEASE`: release name; otherwise the Rust SDK release default is used.
- `SENTRY_TRACES_SAMPLE_RATE`: defaults to `0.0`, or `1.0` when Spotlight is enabled.
- `SENTRY_ENABLE_LOGS`: defaults to `false`, or `true` when Spotlight is enabled.
- `SENTRY_SPOTLIGHT=true`: mirror Sentry envelopes to local Spotlight.
- `SENTRY_SPOTLIGHT_URL`: defaults to `http://localhost:8969/stream`.

Local Spotlight smoke test:

```bash
npx -y @spotlightjs/spotlight server --port 8969 --debug
env -u SENTRY_DSN SENTRY_SPOTLIGHT=true RUNTIME_BIND_ADDR=127.0.0.1:7089 cargo run -p employee-bridge
source .env
curl -H "Authorization: Bearer $RUNTIME_SECRET" \
  -H "content-type: application/json" \
  -d '{"message":"employee-bridge spotlight smoke test"}' \
  http://127.0.0.1:7089/debug/sentry-test
```
