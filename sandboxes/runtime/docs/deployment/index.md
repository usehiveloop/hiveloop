# Deployment

Deploy Bridge to production.

---

## Deployment Options

| Method | Best For |
|--------|----------|
| [Binary](binary-deployment.md) | Single server, simple setup |
| [Docker](docker-deployment.md) | Containers, consistent environments |
| [Kubernetes](kubernetes.md) | Orchestrated, scalable deployments |

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
- [ ] Configure `BRIDGE_DRAIN_TIMEOUT_SECS`
- [ ] Set `BRIDGE_MAX_CONCURRENT_CONVERSATIONS` limit

### Monitoring

- [ ] Health check endpoint configured
- [ ] Metrics collection set up
- [ ] Log aggregation configured
- [ ] Alerting for errors

### Operations

- [ ] Push agents on startup
- [ ] Handle webhooks for persistence
- [ ] Backup strategy for control plane data
- [ ] Runbook for common issues

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

## Read More

- [Binary Deployment](binary-deployment.md) — Run the executable
- [Docker Deployment](docker-deployment.md) — Container deployment
- [Kubernetes](kubernetes.md) — K8s manifests and Helm
- [Monitoring](monitoring.md) — Metrics and observability
