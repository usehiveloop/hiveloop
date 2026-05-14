# Deployment

Deploy Bridge to production.

---

## Deployment Options

| Method | Best For | Binary Size |
|--------|----------|-------------|
| [Binary](binary-deployment.md) | Single server, simple setup | ~10 MB |
| [Docker](docker-deployment.md) | Containers, consistent environments | ~30 MB base + binary |
| [Kubernetes](kubernetes.md) | Orchestrated, scalable deployments | Same as Docker |

---

## Production Checklist

Before deploying to production:

### Security

- [ ] Set strong `BRIDGE_CONTROL_PLANE_API_KEY`
- [ ] Use HTTPS for all endpoints
- [ ] Configure webhook secrets
- [ ] Set up firewall rules
- [ ] Run Bridge as non-root user

### Configuration

- [ ] Set `BRIDGE_LOG_FORMAT=json` for parsing
- [ ] Set `BRIDGE_LOG_LEVEL=info` (or `warn`)
- [ ] Configure `BRIDGE_DRAIN_TIMEOUT_SECS` (default: 60s)
- [ ] Set `BRIDGE_MAX_CONCURRENT_CONVERSATIONS` limit if needed
- [ ] Configure `BRIDGE_LISTEN_ADDR` (default: `0.0.0.0:8080`)

### Monitoring

- [ ] Health check endpoint (`/health`) configured
- [ ] Metrics collection (`/metrics`) set up
- [ ] Log aggregation configured
- [ ] Alerting for errors

### Operations

- [ ] Push agents on startup
- [ ] Handle webhooks for persistence
- [ ] Backup strategy for control plane data
- [ ] Runbook for common issues

---

## Configuration Reference

Bridge can be configured via:

1. **Config file:** `config.toml` in working directory
2. **Environment variables:** Prefixed with `BRIDGE_`

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BRIDGE_CONTROL_PLANE_URL` | `""` | Control plane API URL |
| `BRIDGE_CONTROL_PLANE_API_KEY` | `""` | API key for authentication |
| `BRIDGE_LISTEN_ADDR` | `0.0.0.0:8080` | HTTP listen address |
| `BRIDGE_DRAIN_TIMEOUT_SECS` | `60` | Graceful shutdown timeout |
| `BRIDGE_MAX_CONCURRENT_CONVERSATIONS` | unlimited | Per-agent conversation limit |
| `BRIDGE_LOG_LEVEL` | `info` | Log level (trace/debug/info/warn/error) |
| `BRIDGE_LOG_FORMAT` | `text` | Log format: `text` or `json` |
| `BRIDGE_WEBHOOK_URL` | none | Webhook endpoint URL |

### Config File Example

```toml
control_plane_url = "https://api.example.com"
control_plane_api_key = "<bridge-control-plane-api-key>"
listen_addr = "0.0.0.0:8080"
drain_timeout_secs = 60
log_level = "info"
log_format = "json"
webhook_url = "https://hooks.example.com/events"

[lsp]
rust = { command = ["rust-analyzer"], extensions = ["rs"] }
```

---

## Architecture Patterns

### Single Instance

Simplest deployment. One Bridge process handles all traffic.

```
Users ──► Load Balancer ──► Bridge
```

Good for: Getting started, low traffic

### Multiple Instances

Run multiple Bridge instances behind a load balancer.

```
Users ──► Load Balancer ──► Bridge-1
                      ├──► Bridge-2
                      └──► Bridge-3
```

Requirements:
- Push agents to all instances
- Sticky sessions for conversations
- Shared webhook endpoint

Good for: High availability, horizontal scaling

---

## Health and Metrics

### Health Endpoint

```bash
curl http://localhost:8080/health
```

Returns:
```json
{
  "status": "ok",
  "uptime_secs": 3600
}
```

### Metrics Endpoint

```bash
curl http://localhost:8080/metrics
```

Returns JSON with per-agent and global metrics. See [Monitoring](monitoring.md) for details.

---

## Read More

- [Binary Deployment](binary-deployment.md) — Run the executable
- [Docker Deployment](docker-deployment.md) — Container deployment
- [Kubernetes](kubernetes.md) — K8s manifests and Helm
- [Monitoring](monitoring.md) — Metrics and observability
