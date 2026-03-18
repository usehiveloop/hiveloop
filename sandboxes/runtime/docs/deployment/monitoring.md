# Monitoring

Monitor Bridge in production.

---

## Health Endpoint

Bridge provides a health check at `GET /health`:

```bash
curl http://localhost:8080/health
```

Response:

```json
{
  "status": "ok",
  "uptime_secs": 3600
}
```

Use for load balancer health checks and Kubernetes probes.

**Response fields:**
- `status`: Always `"ok"` for healthy (HTTP 200)
- `uptime_secs`: Seconds since the bridge process started

---

## Metrics Endpoint

Bridge provides operational metrics at `GET /metrics` in JSON format:

```bash
curl http://localhost:8080/metrics
```

Example response:

```json
{
  "timestamp": "2026-01-15T10:30:00Z",
  "agents": [
    {
      "agent_id": "greeter",
      "agent_name": "Greeter Agent",
      "input_tokens": 15234,
      "output_tokens": 8932,
      "total_tokens": 24166,
      "total_requests": 45,
      "failed_requests": 2,
      "active_conversations": 3,
      "total_conversations": 12,
      "tool_calls": 28,
      "avg_latency_ms": 1250.5
    }
  ],
  "global": {
    "total_agents": 1,
    "total_active_conversations": 3,
    "uptime_secs": 3600
  }
}
```

### Available Metrics

**Per-agent metrics:**

| Field | Type | Description |
|-------|------|-------------|
| `agent_id` | string | Unique agent identifier |
| `agent_name` | string | Human-readable agent name |
| `input_tokens` | integer | Total input tokens consumed |
| `output_tokens` | integer | Total output tokens generated |
| `total_tokens` | integer | Sum of input + output tokens |
| `total_requests` | integer | Total LLM requests made |
| `failed_requests` | integer | Number of failed requests |
| `active_conversations` | integer | Currently active conversations |
| `total_conversations` | integer | Total conversations ever created |
| `tool_calls` | integer | Total tool calls executed |
| `avg_latency_ms` | float | Average LLM request latency in milliseconds |

**Global metrics:**

| Field | Type | Description |
|-------|------|-------------|
| `total_agents` | integer | Number of loaded agents |
| `total_active_conversations` | integer | Active conversations across all agents |
| `uptime_secs` | integer | Seconds since bridge started |

**Note:** This endpoint returns JSON, not Prometheus format. For Prometheus integration, use a JSON exporter or ingest via your metrics pipeline.

---

## Logging

### Text Format (Development)

Default format for human readability:

```
INFO  bridge::api > Request: POST /agents/greeter/conversations
INFO  bridge::runtime > Conversation created: conv-abc123
```

### JSON Format (Production)

Enable structured JSON logging:

```bash
export BRIDGE_LOG_FORMAT=json
```

JSON logs are output via tracing-subscriber and include:
- `timestamp`: ISO 8601 timestamp
- `level`: Log level (INFO, WARN, ERROR, DEBUG, TRACE)
- `target`: Rust module path
- `fields`: Structured key-value fields

Example JSON log output:

```json
{"timestamp":"2026-01-15T10:30:00.123456Z","level":"INFO","target":"bridge::api","fields":{"message":"Request: POST /agents/greeter/conversations","agent_id":"greeter"}}
```

**Note:** The exact JSON format depends on the tracing-subscriber version. Use `jq` or a log aggregator to parse.

---

## Common Alerts

### High Error Rate

```yaml
alert: BridgeHighErrorRate
expr: |
  sum(
    rate(bridge_failed_requests_total[5m])
  ) / sum(
    rate(bridge_requests_total[5m])
  ) > 0.1
for: 5m
labels:
  severity: warning
annotations:
  summary: "Bridge error rate is high (> 10%)"
```

### High Latency

Monitor via metrics endpoint or external probing:

```yaml
alert: BridgeHighLatency
expr: bridge_avg_latency_ms > 5000
for: 5m
labels:
  severity: warning
annotations:
  summary: "Bridge average latency > 5s"
```

### Too Many Conversations

```yaml
alert: BridgeHighConversations
expr: bridge_active_conversations > 1000
for: 5m
labels:
  severity: warning
annotations:
  summary: "Bridge has many active conversations"
```

---

## Dashboards

### Grafana

Ingest metrics from the `/metrics` JSON endpoint. Key panels:

- Request rate (requests/second)
- Error rate (%)
- Latency (avg from `avg_latency_ms`)
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
