# HashiCorp Vault Production Setup for HiveLoop

This guide covers setting up HashiCorp Vault in production for envelope encryption with HiveLoop.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Deployment Options](#deployment-options)
3. [Production Configuration](#production-configuration)
4. [Security Hardening](#security-hardening)
5. [Backup and Disaster Recovery](#backup-and-disaster-recovery)
6. [Monitoring](#monitoring)
7. [Application Integration](#application-integration)

---

## Architecture Overview

### Recommended Production Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         AWS/GCP/Azure                           │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    Vault Cluster                        │   │
│  │  ┌─────────┐    ┌─────────┐    ┌─────────┐             │   │
│  │  │ Vault 1 │◄──►│ Vault 2 │◄──►│ Vault 3 │   (HA Mode)  │   │
│  │  │(Leader) │    │(Standby)│    │(Standby)│              │   │
│  │  └────┬────┘    └────┬────┘    └────┬────┘             │   │
│  │       └─────────────────┬─────────────────┘              │   │
│  │                         │                                │   │
│  │              ┌──────────┴──────────┐                     │   │
│  │              │   Consul Backend    │   (or Raft)         │   │
│  │              │   (3-5 nodes)       │                     │   │
│  │              └─────────────────────┘                     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│  ┌───────────────────────────┼─────────────────────────────┐   │
│  │        Private Subnet     │                             │   │
│  │  ┌──────────────────────┐ │  ┌──────────────────────┐   │   │
│  │  │   HiveLoop ECS/EKS   │◄┘  │   HiveLoop ECS/EKS   │   │   │
│  │  │   (Task 1)           │    │   (Task 2)           │   │   │
│  │  └──────────────────────┘    └──────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Deployment Options

### Option 1: HashiCorp Cloud Platform (HCP) Vault (Recommended for Simplicity)

**Best for:** Teams wanting managed Vault without operational overhead

```hcl
# No infrastructure to manage
# Fully managed by HashiCorp
# Automatic upgrades, backups, monitoring
```

**Setup:**
1. Sign up at [portal.cloud.hashicorp.com](https://portal.cloud.hashicorp.com)
2. Create a Vault cluster in your preferred region
3. Enable Transit engine via UI or API
4. Create `hiveloop-key` encryption key
5. Create AppRole auth for HiveLoop

**Pros:**
- Zero operational overhead
- Built-in HA and backups
- SOC 2, HIPAA compliant
- Automatic updates

**Cons:**
- Higher cost (~$0.50/hour for starter)
- Less control over underlying infrastructure

---

### Option 2: Self-Managed Vault on EC2/EKS (Recommended for Control)

**Best for:** Teams needing full control, compliance requirements

#### Terraform Configuration

```hcl
# modules/vault/main.tf

# IAM Role for Vault nodes
resource "aws_iam_role" "vault" {
  name = "${var.name_prefix}-vault-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
    }]
  })
}

# KMS key for auto-unseal (critical for production!)
resource "aws_kms_key" "vault_unseal" {
  description             = "Vault auto-unseal key"
  deletion_window_in_days = 30
  enable_key_rotation     = true

  tags = {
    Name = "${var.name_prefix}-vault-unseal"
  }
}

# Security group
resource "aws_security_group" "vault" {
  name_prefix = "${var.name_prefix}-vault-"
  vpc_id      = var.vpc_id

  # Vault API - restrict to application security group ONLY
  ingress {
    from_port       = 8200
    to_port         = 8200
    protocol        = "tcp"
    security_groups = [var.app_security_group_id]
  }

  # Vault cluster communication
  ingress {
    from_port = 8201
    to_port   = 8201
    protocol  = "tcp"
    self      = true
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# Launch template for Vault nodes
resource "aws_launch_template" "vault" {
  name_prefix   = "${var.name_prefix}-vault-"
  image_id      = data.aws_ami.amazon_linux_2023.id
  instance_type = "t3.medium"
  
  iam_instance_profile {
    name = aws_iam_instance_profile.vault.name
  }

  vpc_security_group_ids = [aws_security_group.vault.id]

  user_data = base64encode(templatefile("${path.module}/vault-userdata.sh", {
    kms_key_id = aws_kms_key.vault_unseal.id
    region     = var.aws_region
  }))

  tag_specifications {
    resource_type = "instance"
    tags = {
      Name = "${var.name_prefix}-vault"
    }
  }
}

# Auto Scaling Group for HA
resource "aws_autoscaling_group" "vault" {
  name                = "${var.name_prefix}-vault"
  desired_capacity    = 3
  min_size            = 3
  max_size            = 5
  vpc_zone_identifier = var.private_subnet_ids

  launch_template {
    id      = aws_launch_template.vault.id
    version = "$Latest"
  }

  target_group_arns = [aws_lb_target_group.vault.arn]

  health_check_type         = "ELB"
  health_check_grace_period = 300

  tag {
    key                 = "Name"
    value               = "${var.name_prefix}-vault"
    propagate_at_launch = true
  }
}

# Application Load Balancer
resource "aws_lb" "vault" {
  name               = "${var.name_prefix}-vault"
  internal           = true  # Internal only - no public access!
  load_balancer_type = "application"
  security_groups    = [aws_security_group.vault_lb.id]
  subnets            = var.private_subnet_ids
}

resource "aws_lb_target_group" "vault" {
  name     = "${var.name_prefix}-vault"
  port     = 8200
  protocol = "HTTPS"
  vpc_id   = var.vpc_id

  health_check {
    path     = "/v1/sys/health"
    port     = 8200
    protocol = "HTTPS"
    matcher  = "200,429,472,473"  # Vault health codes
  }
}
```

#### Vault Configuration File

```hcl
# /etc/vault.d/vault.hcl

disclaimer = "This configuration is for production use"

# Listener with TLS (REQUIRED for production!)
listener "tcp" {
  address       = "0.0.0.0:8200"
  tls_cert_file = "/opt/vault/tls/vault.crt"
  tls_key_file  = "/opt/vault/tls/vault.key"
  
  # TLS configuration
  tls_min_version = "tls13"
  tls_cipher_suites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256"
  
  # Require client certs for additional security (optional)
  # tls_require_and_verify_client_cert = true
  # tls_client_ca_file = "/opt/vault/tls/ca.crt"
}

# Storage backend - Raft for integrated storage (recommended)
storage "raft" {
  path    = "/opt/vault/data"
  node_id = "${NODE_ID}"

  retry_leader_election = true

  # Peers will be auto-joined via auto-join
}

# Auto-join for cluster formation
retry_join {
  auto_join = "provider=aws region=${AWS_REGION} tag_key=Name tag_value=${CLUSTER_TAG}"
  auto_join_scheme = "https"
}

# AWS KMS Auto-Unseal (REQUIRED for production!)
seal "awskms" {
  region     = "${AWS_REGION}"
  kms_key_id = "${KMS_KEY_ID}"
}

# Telemetry for monitoring
telemetry {
  statsite_address = "statsite.service.consul:8125"
  disable_hostname = true
  
  # Prometheus metrics
  prometheus_retention_time = "30s"
  disable_hostname          = true
}

# Logging
log_level = "warn"
log_format = "json"

# API settings
api_addr     = "https://${INSTANCE_IP}:8200"
cluster_addr = "https://${INSTANCE_IP}:8201"

# Performance tuning
default_lease_ttl = "768h"  # 32 days
max_lease_ttl     = "768h"

# UI (disable in production if not needed, or restrict access)
ui = true
```

---

### Option 3: Vault on Kubernetes (EKS) with Helm

**Best for:** Teams already using Kubernetes

```yaml
# vault-values.yaml
server:
  image:
    repository: hashicorp/vault
    tag: "1.18.0"
  
  # HA configuration with Raft
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
  
  # Resources
  resources:
    requests:
      memory: "512Mi"
      cpu: "500m"
    limits:
      memory: "1Gi"
      cpu: "1000m"
  
  # Security contexts
  securityContext:
    runAsNonRoot: true
    runAsUser: 100
    runAsGroup: 1000
    fsGroup: 1000
  
  # Affinity rules
  affinity: |
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              app.kubernetes.io/name: {{ template "vault.name" . }}
              app.kubernetes.io/instance: "{{ .Release.Name }}"
              component: server
          topologyKey: kubernetes.io/hostname
  
  # Data volume
  dataStorage:
    enabled: true
    size: 10Gi
    storageClass: "gp3-encrypted"
  
  # Audit storage
  auditStorage:
    enabled: true
    size: 10Gi
    storageClass: "gp3-encrypted"

# Injector for Kubernetes auth
injector:
  enabled: true
  replicas: 2
```

**Deploy:**
```bash
# Add HashiCorp Helm repo
helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo update

# Create namespace
kubectl create namespace vault

# Create TLS secret (from cert-manager or manually)
kubectl create secret tls vault-server-tls \
  --cert=vault.crt \
  --key=vault.key \
  -n vault

# Install Vault
helm install vault hashicorp/vault \
  -n vault \
  -f vault-values.yaml

# Initialize and unseal (only needed without auto-unseal)
kubectl exec -it vault-0 -n vault -- vault operator init
```

---

## Production Configuration

### 1. Enable Transit Engine with Proper ACLs

```bash
# Login to Vault
export VAULT_ADDR="https://vault.internal.yourcompany.com:8200"
vault login -method=userpass username=admin

# Enable Transit engine
vault secrets enable -path=transit transit

# Create the encryption key for HiveLoop
vault write -f transit/keys/hiveloop-key

# Enable automatic key rotation (recommended)
vault write transit/keys/hiveloop-key/config \
  auto_rotate_period=768h  # Rotate every 32 days

# Verify key
vault read transit/keys/hiveloop-key
```

### 2. Create Vault Policies for HiveLoop

```hcl
# hiveloop-policy.hcl

# Read key metadata (needed for operations)
path "transit/keys/hiveloop-key" {
  capabilities = ["read"]
}

# Encrypt DEKs
path "transit/encrypt/hiveloop-key" {
  capabilities = ["update"]
}

# Decrypt DEKs
path "transit/decrypt/hiveloop-key" {
  capabilities = ["update"]
}

# Generate data keys (optional - if you want Vault to generate DEKs)
path "transit/datakey/plaintext/hiveloop-key" {
  capabilities = ["update"]
}

# Rotate key (if you want application to trigger rotation)
path "transit/keys/hiveloop-key/rotate" {
  capabilities = ["update"]
}
```

```bash
# Create the policy
vault policy write hiveloop-production hiveloop-policy.hcl
```

### 3. Configure Authentication

#### Option A: AppRole (Recommended for ECS/EC2)

```bash
# Enable AppRole auth
vault auth enable approle

# Create AppRole for HiveLoop
vault write auth/approle/role/hiveloop \
  token_policies="hiveloop-production" \
  token_ttl=1h \
  token_max_ttl=4h \
  secret_id_ttl=24h \
  secret_id_num_uses=0

# Get RoleID
vault read auth/approle/role/hiveloop/role-id

# Generate SecretID
vault write -f auth/approle/role/hiveloop/secret-id
```

**Application configuration:**
```bash
KMS_TYPE=vault
KMS_KEY=hiveloop-key
VAULT_ADDRESS=https://vault.internal.yourcompany.com:8200
VAULT_AUTH_METHOD=approle
VAULT_ROLE_ID=your-role-id
VAULT_SECRET_ID=your-secret-id
```

#### Option B: Kubernetes Auth (for EKS)

```bash
# Enable Kubernetes auth
vault auth enable kubernetes

# Configure Kubernetes auth
vault write auth/kubernetes/config \
  kubernetes_host="https://$KUBERNETES_PORT_443_TCP_ADDR:443" \
  token_reviewer_jwt="$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" \
  kubernetes_ca_cert=@/var/run/secrets/kubernetes.io/serviceaccount/ca.crt

# Create role for HiveLoop service account
vault write auth/kubernetes/role/hiveloop \
  bound_service_account_names=hiveloop \
  bound_service_account_namespaces=production \
  policies=hiveloop-production \
  ttl=1h
```

**Deploy with Vault Agent Injector:**
```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hiveloop
spec:
  template:
    metadata:
      annotations:
        vault.hashicorp.com/agent-inject: "true"
        vault.hashicorp.com/role: "hiveloop"
        vault.hashicorp.com/agent-inject-secret-vault-token: "auth/token/lookup-self"
        vault.hashicorp.com/agent-inject-template-vault-token: |
          {{ with secret "auth/token/lookup-self" }}{{ .Data.id }}{{ end }}
    spec:
      serviceAccountName: hiveloop
      containers:
      - name: hiveloop
        env:
        - name: VAULT_TOKEN
          value: "/vault/secrets/vault-token"
        - name: VAULT_ADDRESS
          value: "http://vault.vault.svc:8200"
```

#### Option C: AWS IAM Auth (Best for ECS on AWS)

```bash
# Enable AWS auth
vault auth enable aws

# Configure AWS auth
vault write auth/aws/config/client \
  iam_server_id_header_value="vault.yourcompany.com"

# Create role
vault write auth/aws/role/hiveloop-ecs \
  auth_type=iam \
  bound_iam_principal_arn="arn:aws:iam::ACCOUNT:role/hiveloop-ecs-task-role" \
  policies=hiveloop-production \
  ttl=1h
```

---

## Security Hardening

### 1. Network Security

```bash
# Vault should NEVER be publicly accessible!

# Security group rules:
# - Ingress: Port 8200 from application security group ONLY
# - Ingress: Port 8201 (cluster) from Vault nodes only
# - Egress: Required for KMS, storage backends

# Use AWS PrivateLink or VPC peering for cross-VPC access
```

### 2. TLS Configuration

```bash
# Use certificates from ACM PCA or cert-manager
# Minimum TLS 1.2 (1.3 recommended)

# Certificate requirements:
# - SAN must include vault.internal.yourcompany.com
# - Use private CA for internal services
# - Rotate certificates before expiry
```

### 3. Audit Logging

```bash
# Enable audit device
vault audit enable file file_path=/var/log/vault/audit.log

# Or send to SIEM (Splunk, Datadog, etc.)
vault audit enable socket address=siem.internal:5140 socket_type=tcp

# Audit logs contain:
# - All encryption/decryption operations
# - Key access patterns
# - Authentication events
```

### 4. Secrets Encryption at Rest

```bash
# Vault already encrypts data at rest
# For additional security, use encrypted EBS volumes

# AWS - enable encryption
aws ec2 create-volume \
  --size 100 \
  --region us-east-1 \
  --availability-zone us-east-1a \
  --encrypted \
  --kms-key-id alias/vault-storage
```

---

## Backup and Disaster Recovery

### 1. Automated Snapshots (Raft Storage)

```bash
# Configure automated snapshots
vault write sys/storage/raft/snapshot-auto/config/hourly \
  interval=1h \
  retain=168 \
  storage_type=aws-s3 \
  aws_s3_bucket=vault-snapshots-yourcompany \
  aws_s3_region=us-east-1

# Cross-region replication for the S3 bucket!
```

### 2. Manual Backup

```bash
# Create snapshot
vault operator raft snapshot save vault-backup-$(date +%Y%m%d).snap

# Restore (disaster recovery)
vault operator raft snapshot restore vault-backup-20250310.snap
```

### 3. Disaster Recovery Cluster (Enterprise)

```hcl
# Enable DR replication (Vault Enterprise)
replication {
  dr {
    primary {
      cluster_addr = "https://vault-primary.internal:8201"
    }
  }
}
```

---

## Monitoring

### 1. Key Metrics to Monitor

```promql
# Vault is sealed (CRITICAL)
vault_core_unsealed{job="vault"} == 0

# High latency on crypto operations
histogram_quantile(0.99, 
  rate(vault_transit_encrypt_time_seconds_bucket[5m])
) > 0.1

# Authentication failures increasing
rate(vault_core_auth_failure_count[5m]) > 0.1

# Certificate expiry (alert at 30 days)
vault_tls_cert_expiry_seconds / 86400 < 30

# Storage backend health
vault_raft_stats_applied_index_rate < 1
```

### 2. Datadog/NewRelic Integration

```hcl
# telemetry config
telemetry {
  dogstatsd_addr = "localhost:8125"
  dogstatsd_tags = ["env:production", "service:vault"]
}
```

### 3. Log Aggregation

```bash
# Ship audit logs to centralized logging
# Watch for:
# - Unusual DEK unwrap patterns (possible data exfiltration)
# - Failed authentication attempts
# - Key rotation events
```

---

## Application Integration

### Environment Variables for Production

```bash
# Required
export KMS_TYPE=vault
export KMS_KEY=hiveloop-key
export VAULT_ADDRESS=https://vault.internal.yourcompany.com:8200
export VAULT_TOKEN=s.token_from_auth_method

# Optional but recommended
export VAULT_NAMESPACE=""           # Enterprise only
export VAULT_MOUNT_PATH=transit     # If using non-default mount
export VAULT_CA_CERT=/certs/ca.crt  # For private CA

# Timeouts (tune based on your network)
export VAULT_CLIENT_TIMEOUT=30s
export VAULT_MAX_RETRIES=3
```

### Health Check for HiveLoop

```go
// Add to your health check endpoint
func vaultHealthCheck(ctx context.Context, vaultWrapper *crypto.KeyWrapper) error {
	// Try to wrap a test DEK
	testDEK := make([]byte, 32)
	rand.Read(testDEK)
	
	wrapped, err := vaultWrapper.Wrap(ctx, testDEK)
	if err != nil {
		return fmt.Errorf("vault wrap failed: %w", err)
	}
	
	_, err = vaultWrapper.Unwrap(ctx, wrapped)
	if err != nil {
		return fmt.Errorf("vault unwrap failed: %w", err)
	}
	
	return nil
}
```

### Key Rotation Strategy

```bash
# 1. Rotate Vault Transit key (creates new key version)
vault write -f transit/keys/hiveloop-key/rotate

# 2. Re-encrypt existing DEKs (optional, gradual migration)
# This is typically not needed - old ciphertext decrypts with old key version
# Only new DEKs use the new key version

# 3. Update min_decryption_version to prevent downgrade attacks
vault write transit/keys/hiveloop-key/config \
  min_decryption_version=2

# 4. Monitor for any decryption failures
```

---

## Cost Optimization

| Deployment Type | Monthly Cost (Estimate) | Best For |
|----------------|------------------------|----------|
| HCP Vault Starter | ~$350-400 | Small teams, no ops |
| 3x t3.medium EC2 | ~$200-250 + ops | Medium scale, control needed |
| EKS + 3 pods | ~$150-200 + ops | Already on Kubernetes |
| Vault Enterprise | $$$$ | Large org, DR, namespaces |

---

## Quick Reference: Common Operations

```bash
# Check Vault status
curl https://vault.internal:8200/v1/sys/health

# Check key status
vault read transit/keys/hiveloop-key

# Rotate key
vault write -f transit/keys/hiveloop-key/rotate

# View audit logs
vault audit list

# Check token policies
vault token lookup

# Renew token
vault token renew

# Revoke token (emergency)
vault token revoke -mode=path auth/approle/role/hiveloop

# Seal Vault (emergency - stops all operations!)
vault operator seal

# Unseal (if not using auto-unseal)
vault operator unseal <unseal-key-1>
vault operator unseal <unseal-key-2>
vault operator unseal <unseal-key-3>
```

---

## Troubleshooting

### Issue: Vault is sealed
**Solution:** Check KMS auto-unseal configuration, IAM permissions

### Issue: High latency on encrypt/decrypt
**Solution:** Check network connectivity, consider VPC endpoints, enable connection pooling

### Issue: "permission denied" errors
**Solution:** Verify policy, check token TTL, ensure proper auth method

### Issue: Key not found
**Solution:** Verify Transit engine is enabled at the correct path, key exists

---

## Next Steps

1. **Choose deployment option** based on your infrastructure
2. **Set up TLS certificates** from your internal CA
3. **Configure auto-unseal** with AWS KMS
4. **Create policies** and authentication method
5. **Enable audit logging** to your SIEM
6. **Set up monitoring** dashboards and alerts
7. **Document key rotation** procedures
8. **Test disaster recovery** quarterly

For detailed Vault documentation: [vaultproject.io/docs](https://www.vaultproject.io/docs)
