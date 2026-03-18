# Binary Deployment

Run Bridge as a standalone binary.

---

## Build

Build the release binary:

```bash
cargo build --release
```

The binary is at `target/release/bridge`.

---

## Run

### Minimal

```bash
export BRIDGE_CONTROL_PLANE_API_KEY="<bridge-control-plane-api-key>"
./bridge
```

### With Config File

```bash
./bridge --config /etc/bridge/config.toml
```

### All Options

```bash
export BRIDGE_CONTROL_PLANE_API_KEY="..."
export BRIDGE_LISTEN_ADDR="0.0.0.0:8080"
export BRIDGE_LOG_LEVEL="info"
export BRIDGE_LOG_FORMAT="json"
export BRIDGE_WEBHOOK_URL="https://api.example.com/webhooks"
export BRIDGE_DRAIN_TIMEOUT_SECS="120"

./bridge
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
  "status": "healthy",
  "uptime_seconds": 3600
}
```

Use this for load balancer health checks.

---

## Graceful Shutdown

Bridge handles SIGTERM gracefully:

1. Stops accepting new connections
2. Waits for active conversations to finish (up to `DRAIN_TIMEOUT_SECS`)
3. Exits

---

## See Also

- [Docker Deployment](docker-deployment.md) — Container approach
- [Monitoring](monitoring.md) — Observability
