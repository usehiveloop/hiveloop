# Monitoring

Monitor Bridge in production.

---

## Health Endpoint

Bridge provides a health check:

```bash
curl http://localhost:8080/health
```

Response:

```json
{
  "status": "healthy",
  "uptime_seconds": 3600
}
```

Use for load balancer health checks and Kubernetes probes.

---

## Metrics Endpoint

Prometheus-compatible metrics at `/metrics`:

```bash
curl http://localhost:8080/metrics
```

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `bridge_requests_total` | counter | Total HTTP requests |
| `bridge_request_duration_seconds` | histogram | Request latency |
| `bridge_conversations_active` | gauge | Active conversations |
| `bridge_agents_loaded` | gauge | Loaded agents |
| `bridge_tokens_used_total` | counter | Total tokens used |

---

## Logging

### Text Format (Development)

```
INFO  bridge::api > Request: POST /agents/greeter/conversations
INFO  bridge::runtime > Conversation created: conv-abc123
```

### JSON Format (Production)

```json
{
  "timestamp": "2026-01-15T10:30:00Z",
  "level": "INFO",
  "target": "bridge::api",
  "message": "Request: POST /agents/greeter/conversations",
  "fields": {
    "agent_id": "greeter",
    "conversation_id": "conv-abc123"
  }
}
```

Enable JSON:

```bash
export BRIDGE_LOG_FORMAT=json
```

---

## Common Alerts

### High Error Rate

```yaml
alert: BridgeHighErrorRate
expr: rate(bridge_requests_total{status=~"5.."}[5m]) > 0.1
for: 5m
labels:
  severity: warning
annotations:
  summary: "Bridge error rate is high"
```

### High Latency

```yaml
alert: BridgeHighLatency
expr: histogram_quantile(0.95, rate(bridge_request_duration_seconds_bucket[5m])) > 5
for: 5m
labels:
  severity: warning
annotations:
  summary: "Bridge p95 latency > 5s"
```

### Too Many Conversations

```yaml
alert: BridgeHighConversations
expr: bridge_conversations_active > 1000
for: 5m
labels:
  severity: warning
annotations:
  summary: "Bridge has many active conversations"
```

---

## Dashboards

### Grafana

Import metrics from Prometheus. Key panels:

- Request rate (requests/second)
- Error rate (%)
- Latency (p50, p95, p99)
- Active conversations
- Token usage

---

## Tracing

Bridge doesn't include distributed tracing. Add at your load balancer or control plane if needed.

---

## Debugging

### Enable Debug Logging

```bash
export BRIDGE_LOG_LEVEL=debug
./bridge
```

This logs:
- All HTTP requests
- LLM API calls
- Tool executions
- Webhook deliveries

### Common Issues

| Symptom | Check |
|---------|-------|
| High latency | LLM provider latency, tool timeouts |
| Errors | Check `error` logs, webhook failures |
| Memory growth | Active conversations, compaction settings |
| CPU usage | Concurrent conversations, tool execution |

---

## Log Aggregation

### Vector

```toml
[sources.bridge]
type = "file"
include = ["/var/log/bridge/*.log"]

[sinks.elasticsearch]
type = "elasticsearch"
inputs = ["bridge"]
endpoint = "http://elasticsearch:9200"
```

### Fluentd

```xml
<source>
  @type tail
  path /var/log/bridge/*.log
  pos_file /var/log/bridge.pos
  tag bridge
  <parse>
    @type json
  </parse>
</source>
```

---

## See Also

- [Binary Deployment](binary-deployment.md)
- [Docker Deployment](docker-deployment.md)
- [Kubernetes](kubernetes.md)
