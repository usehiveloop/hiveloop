---
title: Kubernetes Deployment
description: Deploy LLMVault on Kubernetes using Helm or raw manifests
---

# Kubernetes Deployment

This guide covers deploying LLMVault on Kubernetes for production-grade high availability and scalability.

## Prerequisites

- Kubernetes 1.28+
- Helm 3.12+ (optional but recommended)
- kubectl configured with cluster access
- External PostgreSQL (RDS, Cloud SQL, etc.) or in-cluster Postgres
- External Redis (ElastiCache, Memorystore, etc.) or in-cluster Redis
- External KMS (AWS KMS, HashiCorp Vault, or HCP Vault)

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Kubernetes Cluster                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                      Ingress Controller                      │   │
│  │                    (nginx-traefik-istio)                     │   │
│  └─────────────────────────────┬───────────────────────────────┘   │
│                                │                                    │
│  ┌─────────────────────────────▼───────────────────────────────┐   │
│  │                      LLMVault Service                        │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │   │
│  │  │  Pod 1      │  │  Pod 2      │  │  Pod 3      │  HPA    │   │
│  │  │  (Ready)    │  │  (Ready)    │  │  (Ready)    │         │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘         │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                │                                    │
│         ┌──────────────────────┼──────────────────────┐            │
│         ▼                      ▼                      ▼            │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐       │
│  │  PostgreSQL  │     │    Redis     │     │    Vault     │       │
│  │  (External)  │     │  (External)  │     │  (External)  │       │
│  └──────────────┘     └──────────────┘     └──────────────┘       │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Option 1: Helm Chart (Recommended)

### Add LLMVault Helm Repository

```bash
helm repo add llmvault https://charts.llmvault.dev
helm repo update
```

### Create Values File

Create `llmvault-values.yaml`:

```yaml
# ===========================================
# LLMVault Helm Chart Values
# ===========================================

replicaCount: 3

image:
  repository: llmvault/llmvault
  tag: "latest"
  pullPolicy: IfNotPresent

# ===========================================
# Environment Configuration
# ===========================================

environment:
  # Core settings
  ENVIRONMENT: production
  PORT: "8080"
  LOG_LEVEL: info
  LOG_FORMAT: json
  
  # Database (using external RDS)
  DB_HOST: "llmvault-db.cluster-xxx.us-east-1.rds.amazonaws.com"
  DB_PORT: "5432"
  DB_USER: "llmvault"
  DB_NAME: "llmvault"
  DB_SSLMODE: require
  
  # Redis (using ElastiCache)
  REDIS_URL: "rediss://llmvault-cache.xxx.cache.amazonaws.com:6379"
  REDIS_CACHE_TTL: "30m"
  
  # Caching
  MEM_CACHE_TTL: "5m"
  MEM_CACHE_MAX_SIZE: "10000"
  
  # KMS (AWS KMS)
  KMS_TYPE: awskms
  KMS_KEY: "alias/llmvault-production"
  AWS_REGION: "us-east-1"
  
  # Auth (built-in)
  AUTH_ISSUER: "llmvault"
  AUTH_AUDIENCE: "https://api.llmvault.dev"
  
  # OAuth (Nango)
  NANGO_ENDPOINT: "https://integrations.yourcompany.com"
  
  # CORS
  CORS_ORIGINS: "https://vault.yourcompany.com,https://admin.yourcompany.com"

# ===========================================
# Secrets (create separately)
# ===========================================

secrets:
  # Reference to existing secret
  existingSecret: llmvault-secrets

# ===========================================
# Service Configuration
# ===========================================

service:
  type: ClusterIP
  port: 8080
  annotations: {}

# ===========================================
# Ingress Configuration
# ===========================================

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-body-size: "100m"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "600"
  hosts:
    - host: api.yourcompany.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: llmvault-tls
      hosts:
        - api.yourcompany.com

# ===========================================
# Autoscaling
# ===========================================

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 80

# ===========================================
# Resource Limits
# ===========================================

resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 1Gi

# ===========================================
# Health Checks
# ===========================================

livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3

# ===========================================
# Security
# ===========================================

podSecurityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  fsGroup: 1000

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL

# ===========================================
# Monitoring
# ===========================================

serviceMonitor:
  enabled: true
  interval: 30s
  path: /metrics

# ===========================================
# Pod Disruption Budget
# ===========================================

podDisruptionBudget:
  enabled: true
  minAvailable: 2
```

### Create Secrets

```bash
# Create secret with sensitive values
kubectl create namespace llmvault

kubectl create secret generic llmvault-secrets \
  --namespace llmvault \
  --from-literal=DB_PASSWORD="your-db-password" \
  --from-literal=JWT_SIGNING_KEY="$(openssl rand -hex 32)" \
  --from-file=AUTH_RSA_PRIVATE_KEY=certs/auth.key \
  --from-literal=AUTH_ISSUER="llmvault" \
  --from-literal=AUTH_AUDIENCE="https://api.llmvault.dev" \
  --from-literal=NANGO_SECRET_KEY="your-nango-secret-key"
```

### Deploy

```bash
helm install llmvault llmvault/llmvault \
  --namespace llmvault \
  --values llmvault-values.yaml
```

### Verify Deployment

```bash
# Check pods
kubectl get pods -n llmvault

# Check logs
kubectl logs -f deployment/llmvault -n llmvault

# Test endpoint
kubectl port-forward svc/llmvault 8080:8080 -n llmvault
curl http://localhost:8080/health
```

## Option 2: Raw Kubernetes Manifests

### Namespace

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: llmvault
  labels:
    name: llmvault
```

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: llmvault-config
  namespace: llmvault
data:
  ENVIRONMENT: "production"
  PORT: "8080"
  LOG_LEVEL: "info"
  LOG_FORMAT: "json"
  DB_HOST: "llmvault-db.cluster-xxx.us-east-1.rds.amazonaws.com"
  DB_PORT: "5432"
  DB_USER: "llmvault"
  DB_NAME: "llmvault"
  DB_SSLMODE: "require"
  REDIS_URL: "rediss://llmvault-cache.xxx.cache.amazonaws.com:6379"
  REDIS_CACHE_TTL: "30m"
  MEM_CACHE_TTL: "5m"
  MEM_CACHE_MAX_SIZE: "10000"
  KMS_TYPE: "awskms"
  KMS_KEY: "alias/llmvault-production"
  AWS_REGION: "us-east-1"
  AUTH_ISSUER: "llmvault"
  AUTH_AUDIENCE: "https://api.llmvault.dev"
  NANGO_ENDPOINT: "https://integrations.yourcompany.com"
  CORS_ORIGINS: "https://vault.yourcompany.com"
```

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llmvault-secrets
  namespace: llmvault
type: Opaque
stringData:
  DB_PASSWORD: "your-secure-db-password"
  JWT_SIGNING_KEY: "your-jwt-signing-key-min-32-chars"
  AUTH_RSA_PRIVATE_KEY: |
    -----BEGIN PRIVATE KEY-----
    your-rsa-private-key-content
    -----END PRIVATE KEY-----
  AUTH_ISSUER: "llmvault"
  AUTH_AUDIENCE: "https://api.llmvault.dev"
  NANGO_SECRET_KEY: "your-nango-secret-key"
```

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: llmvault
  namespace: llmvault
  labels:
    app: llmvault
spec:
  replicas: 3
  selector:
    matchLabels:
      app: llmvault
  template:
    metadata:
      labels:
        app: llmvault
    spec:
      serviceAccountName: llmvault
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
      containers:
        - name: llmvault
          image: llmvault/llmvault:latest
          imagePullPolicy: IfNotPresent
          ports:
            - name: http
              containerPort: 8080
              protocol: TCP
          envFrom:
            - configMapRef:
                name: llmvault-config
            - secretRef:
                name: llmvault-secrets
          resources:
            requests:
              cpu: 500m
              memory: 512Mi
            limits:
              cpu: 2000m
              memory: 1Gi
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 5
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
            timeoutSeconds: 3
            failureThreshold: 3
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - ALL
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: llmvault
  namespace: llmvault
  annotations:
    # For AWS IAM Roles for Service Accounts (IRSA)
    eks.amazonaws.com/role-arn: arn:aws:iam::ACCOUNT:role/llmvault-kms-role
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: llmvault
  namespace: llmvault
  labels:
    app: llmvault
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
      protocol: TCP
      name: http
  selector:
    app: llmvault
```

### Horizontal Pod Autoscaler

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: llmvault
  namespace: llmvault
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: llmvault
  minReplicas: 3
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

### Pod Disruption Budget

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: llmvault
  namespace: llmvault
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: llmvault
```

## Ingress Configuration

### NGINX Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: llmvault
  namespace: llmvault
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "100m"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "600"
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - api.yourcompany.com
      secretName: llmvault-tls
  rules:
    - host: api.yourcompany.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: llmvault
                port:
                  number: 8080
```

### Traefik Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: llmvault
  namespace: llmvault
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
    traefik.ingress.kubernetes.io/router.middlewares: llmvault-cors@kubernetescrd
spec:
  rules:
    - host: api.yourcompany.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: llmvault
                port:
                  number: 8080
```

## HashiCorp Vault on Kubernetes

### Vault Helm Values

```yaml
server:
  image:
    repository: hashicorp/vault
    tag: "1.18.0"
  
  ha:
    enabled: true
    replicas: 3
    raft:
      enabled: true
      setNodeId: true
      config: |
        ui = true
        
        listener "tcp" {
          tls_disable = 0
          address = "[::]:8200"
          cluster_address = "[::]:8201"
          tls_cert_file = "/vault/userconfig/vault-server-tls/tls.crt"
          tls_key_file = "/vault/userconfig/vault-server-tls/tls.key"
          tls_ca_file = "/vault/userconfig/vault-server-tls/ca.crt"
        }
        
        storage "raft" {
          path = "/vault/data"
        }
        
        seal "awskms" {
          region = "us-east-1"
          kms_key_id = "arn:aws:kms:us-east-1:ACCOUNT:key/KEY-ID"
        }
        
        telemetry {
          prometheus_retention_time = "30s"
          disable_hostname = true
        }
        
        service_registration "kubernetes" {}
  
  resources:
    requests:
      memory: "512Mi"
      cpu: "500m"
    limits:
      memory: "1Gi"
      cpu: "1000m"
  
  dataStorage:
    enabled: true
    size: 10Gi
    storageClass: "gp3-encrypted"
  
  auditStorage:
    enabled: true
    size: 10Gi
    storageClass: "gp3-encrypted"

injector:
  enabled: true
  replicas: 2
```

### Deploy Vault

```bash
# Add HashiCorp Helm repo
helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo update

# Create namespace
kubectl create namespace vault

# Create TLS secret
kubectl create secret tls vault-server-tls \
  --cert=vault.crt \
  --key=vault.key \
  -n vault

# Install Vault
helm install vault hashicorp/vault \
  -n vault \
  -f vault-values.yaml

# Initialize (only needed without auto-unseal)
kubectl exec -it vault-0 -n vault -- vault operator init
```

### Configure Vault for LLMVault

```bash
# Port forward to Vault
kubectl port-forward svc/vault 8200:8200 -n vault

export VAULT_ADDR=https://localhost:8200
export VAULT_SKIP_VERIFY=true

# Login
vault login

# Enable Transit engine
vault secrets enable -path=transit transit

# Create encryption key
vault write -f transit/keys/llmvault-key

# Enable auto-rotation
vault write transit/keys/llmvault-key/config \
  auto_rotate_period=768h

# Create policy
cat > llmvault-policy.hcl <<EOF
path "transit/keys/llmvault-key" {
  capabilities = ["read"]
}

path "transit/encrypt/llmvault-key" {
  capabilities = ["update"]
}

path "transit/decrypt/llmvault-key" {
  capabilities = ["update"]
}
EOF

vault policy write llmvault-production llmvault-policy.hcl

# Enable Kubernetes auth
vault auth enable kubernetes

# Configure Kubernetes auth
vault write auth/kubernetes/config \
  kubernetes_host="https://$KUBERNETES_PORT_443_TCP_ADDR:443" \
  token_reviewer_jwt="$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" \
  kubernetes_ca_cert=@/var/run/secrets/kubernetes.io/serviceaccount/ca.crt

# Create role
vault write auth/kubernetes/role/llmvault \
  bound_service_account_names=llmvault \
  bound_service_account_namespaces=llmvault \
  policies=llmvault-production \
  ttl=1h
```

## AWS IAM Roles for Service Accounts (IRSA)

### IAM Policy for KMS Access

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncryptFrom",
        "kms:ReEncryptTo",
        "kms:GenerateDataKey",
        "kms:GenerateDataKeyWithoutPlaintext",
        "kms:DescribeKey"
      ],
      "Resource": "arn:aws:kms:us-east-1:ACCOUNT:key/KEY-ID"
    }
  ]
}
```

### Trust Policy

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::ACCOUNT:oidc-provider/oidc.eks.us-east-1.amazonaws.com/id/CLUSTER_ID"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "oidc.eks.us-east-1.amazonaws.com/id/CLUSTER_ID:sub": "system:serviceaccount:llmvault:llmvault",
          "oidc.eks.us-east-1.amazonaws.com/id/CLUSTER_ID:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
```

## Upgrading

### Helm Upgrade

```bash
# Update Helm repository
helm repo update

# Upgrade release
helm upgrade llmvault llmvault/llmvault \
  --namespace llmvault \
  --values llmvault-values.yaml \
  --wait

# Rollback if needed
helm rollback llmvault -n llmvault
```

### Rolling Update

```bash
# Update image tag
kubectl set image deployment/llmvault llmvault=llmvault/llmvault:v1.2.3 -n llmvault

# Watch rollout
kubectl rollout status deployment/llmvault -n llmvault

# Rollback if needed
kubectl rollout undo deployment/llmvault -n llmvault
```

## Monitoring

### Prometheus ServiceMonitor

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: llmvault
  namespace: llmvault
  labels:
    app: llmvault
spec:
  selector:
    matchLabels:
      app: llmvault
  endpoints:
    - port: http
      interval: 30s
      path: /metrics
```

### Grafana Dashboard

Key metrics to monitor:

- Request rate and latency
- Error rates
- Cache hit/miss rates
- Database connection pool
- KMS operation latency

## Related Guides

- [Docker Compose Deployment](./docker-compose) - For simpler deployments
- [Configuration Reference](./configuration) - Detailed configuration options
- [Environment Variables](./environment) - Complete environment variable reference
