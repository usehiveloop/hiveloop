# Binary Deployment

Run Bridge as a standalone binary.

---

## Build

Build the release binary:

```bash
cargo build --release
```

The binary is at `target/release/bridge`.

**Binary size:** ~10 MB (release build with optimizations)

---

## Run

### Minimal

```bash
export BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>"
./bridge
```

### With Config File

Bridge looks for `config.toml` in the working directory:

```bash
cd /etc/bridge
./bridge
```

Or use environment variables (which override config file values):

### All Options

```bash
export BRIDGE_CONTROL_PLANE_API_KEY="..."
export BRIDGE_LISTEN_ADDR="0.0.0.0:8080"
export BRIDGE_LOG_LEVEL="info"
export BRIDGE_LOG_FORMAT="json"
export BRIDGE_WEBHOOK_URL="https://api.example.com/webhooks"
export BRIDGE_DRAIN_TIMEOUT_SECS="60"

./bridge
```

---

## CLI Commands

Bridge includes a CLI for inspecting and managing the runtime:

### List Available Tools

```bash
./bridge tools list --json
```

Outputs a JSON array of all built-in tools with their schemas:

```json
[
  {
    "name": "Read",
    "description": "Reads a file from the local filesystem...",
    "category": "filesystem",
    "parameters": {
      "$schema": "http://json-schema.org/draft-07/schema#",
      "properties": {
        "filePath": { "type": "string" }
      },
      "required": ["filePath"]
    }
  }
]
```

### View Help

```bash
./bridge --help
./bridge tools --help
```

---

## Systemd Service

Create `/etc/systemd/system/bridge.service`:

```ini
[Unit]
Description=Bridge AI Runtime
After=network.target

[Service]
Type=simple
User=bridge
Group=bridge

Environment="BRIDGE_CONTROL_PLANE_API_KEY=<bridge-control-plane-api-key>"
Environment="BRIDGE_LOG_FORMAT=json"
Environment="BRIDGE_LOG_LEVEL=info"
Environment="BRIDGE_WEBHOOK_URL=https://api.example.com/webhooks"
Environment="BRIDGE_DRAIN_TIMEOUT_SECS=60"

ExecStart=/usr/local/bin/bridge
ExecStop=/bin/kill -SIGTERM $MAINPID
TimeoutStopSec=120

Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable bridge
sudo systemctl start bridge
sudo systemctl status bridge
```

---

## User Setup

Create a dedicated user:

```bash
sudo useradd -r -s /bin/false bridge
sudo mkdir -p /var/lib/bridge
sudo chown bridge:bridge /var/lib/bridge
```

Install the binary:

```bash
sudo cp target/release/bridge /usr/local/bin/
sudo chmod +x /usr/local/bin/bridge
```

---

## Log Management

### Journald (with systemd)

View logs:

```bash
sudo journalctl -u bridge -f
```

### Log Rotation

Create `/etc/logrotate.d/bridge`:

```
/var/log/bridge/*.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    create 0644 bridge bridge
}
```

---

## Reverse Proxy (nginx)

```nginx
upstream bridge {
    server 127.0.0.1:8080;
}

server {
    listen 443 ssl;
    server_name bridge.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://bridge;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        
        # SSE support
        proxy_buffering off;
        proxy_cache off;
    }
}
```

---

## Health Checks

Bridge provides a `/health` endpoint:

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

Use this for load balancer health checks.

**Note:** The `status` field is `"ok"` when healthy. The `uptime_secs` field shows seconds since startup.

---

## Graceful Shutdown

Bridge handles SIGTERM and SIGINT gracefully:

1. Stops accepting new connections
2. Signals cancellation to all running conversations
3. Waits for active conversations to finish (up to `DRAIN_TIMEOUT_SECS`, default: 60s)
4. Disconnects MCP servers
5. Shuts down LSP servers (if enabled)
6. Exits

Configure the drain timeout:

```bash
export BRIDGE_DRAIN_TIMEOUT_SECS=120  # Wait up to 2 minutes
```

**Systemd integration:** Set `TimeoutStopSec` to at least `DRAIN_TIMEOUT_SECS + 10` seconds to allow for cleanup.

---

## See Also

- [Docker Deployment](docker-deployment.md) — Container approach
- [Monitoring](monitoring.md) — Observability
