# Kubernetes Deployment

Deploy Bridge to Kubernetes.

---

## Basic Deployment

Create `deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bridge
  labels:
    app: bridge
spec:
  replicas: 3
  selector:
    matchLabels:
      app: bridge
  template:
    metadata:
      labels:
        app: bridge
    spec:
      containers:
      - name: bridge
        image: bridge:latest
        ports:
        - containerPort: 8080
        env:
        - name: BRIDGE_CONTROL_PLANE_API_KEY
          valueFrom:
            secretKeyRef:
              name: bridge-secrets
              key: api-key
        - name: BRIDGE_LOG_FORMAT
          value: "json"
        - name: BRIDGE_WEBHOOK_URL
          value: "https://api.example.com/webhooks"
        - name: BRIDGE_DRAIN_TIMEOUT_SECS
          value: "60"
        resources:
          limits:
            cpu: "2"
            memory: "2Gi"
          requests:
            cpu: "500m"
            memory: "512Mi"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 3
          failureThreshold: 3
        lifecycle:
          preStop:
            exec:
              command: ["/bin/sh", "-c", "sleep 10"]
        terminationGracePeriodSeconds: 90
```

**Key configuration:**
- **Resources:** CPU 500m-2000m, Memory 512Mi-2Gi (adjust based on workload)
- **Health checks:** `/health` returns `{"status": "ok", "uptime_secs": N}`
- **Graceful shutdown:** `terminationGracePeriodSeconds` should be > `DRAIN_TIMEOUT_SECS + 10`
- **PreStop hook:** Adds 10s delay to allow load balancer to remove pod from rotation

---

## Service

Create `service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: bridge
spec:
  selector:
    app: bridge
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
```

---

## Secrets

Create `secrets.yaml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: bridge-secrets
type: Opaque
stringData:
  api-key: "<bridge-control-plane-api-key>"
```

Apply:

```bash
kubectl apply -f secrets.yaml
```

---

## Ingress

Create `ingress.yaml`:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: bridge
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt"
spec:
  tls:
  - hosts:
    - bridge.example.com
    secretName: bridge-tls
  rules:
  - host: bridge.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: bridge
            port:
              number: 80
```

---

## ConfigMap

For non-sensitive configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: bridge-config
data:
  BRIDGE_LOG_LEVEL: "info"
  BRIDGE_LOG_FORMAT: "json"
  BRIDGE_DRAIN_TIMEOUT_SECS: "60"
```

Use in deployment:

```yaml
envFrom:
- configMapRef:
    name: bridge-config
```

---

## Horizontal Pod Autoscaler

Scale based on CPU:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: bridge
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: bridge
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

---

## Rolling Updates

Update with zero downtime:

```bash
# Update image
kubectl set image deployment/bridge bridge=bridge:v2

# Watch rollout
kubectl rollout status deployment/bridge

# Rollback if needed
kubectl rollout undo deployment/bridge
```

---

## Pod Disruption Budget

Ensure availability during disruptions:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: bridge
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: bridge
```

---

## Monitoring

### Prometheus with JSON Exporter

Bridge's `/metrics` endpoint returns JSON, not Prometheus format. Use json_exporter:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: json-exporter-config
data:
  config.yml: |
    modules:
      bridge:
        metrics:
        - name: bridge_active_conversations
          valuetype: gauge
          help: Active conversations
          path: "{ .global.total_active_conversations }"
        - name: bridge_uptime_seconds
          valuetype: gauge
          help: Bridge uptime
          path: "{ .global.uptime_secs }"
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: bridge
spec:
  selector:
    matchLabels:
      app: bridge
  endpoints:
  - port: http
    path: /metrics
```

### Custom Metrics Endpoint

For direct ingestion, poll `/metrics` and extract fields:

```json
{
  "timestamp": "2026-01-15T10:30:00Z",
  "agents": [...],
  "global": {
    "total_agents": 5,
    "total_active_conversations": 42,
    "uptime_secs": 86400
  }
}
```

---

## Helm Chart

Create a simple Helm chart structure:

```
bridge/
├── Chart.yaml
├── values.yaml
└── templates/
    ├── deployment.yaml
    ├── service.yaml
    ├── ingress.yaml
    └── secrets.yaml
```

Install:

```bash
helm install bridge ./bridge
```

---

## Graceful Shutdown Handling

Bridge properly handles SIGTERM during pod termination:

1. Kubernetes sends SIGTERM to the container
2. Bridge stops accepting new connections
3. Waits up to `DRAIN_TIMEOUT_SECS` for active conversations
4. Kubernetes waits `terminationGracePeriodSeconds` before SIGKILL

**Important:** Set `terminationGracePeriodSeconds` higher than `BRIDGE_DRAIN_TIMEOUT_SECS` to ensure graceful shutdown.

---

## See Also

- [Docker Deployment](docker-deployment.md) — Simpler container option
- [Monitoring](monitoring.md) — Observability setup
