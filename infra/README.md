# LLMVault Infrastructure

AWS CDK project for deploying LLMVault. Two environments, two deployment modes.

| Environment | Mode | Services | Cost |
|-------------|------|----------|------|
| **Production** | ECS Fargate + RDS + ElastiCache | ALB, 3 Fargate services, managed Postgres, managed Redis | ~$75/mo |
| **Staging** | EC2 + Docker Compose + Caddy | Single instance running everything | ~$14/mo |

## Architecture

**Production (Fargate):**
```
Cloudflare DNS
  ├─ llmvault.dev       → CNAME → ALB
  ├─ api.llmvault.dev   → CNAME → ALB
  ├─ auth.llmvault.dev  → CNAME → ALB
  └─ connect.llmvault.dev → CNAME → CloudFront

ALB (host-based routing)
  ├─ api.llmvault.dev   → Fargate: Go API (port 8080)
  ├─ llmvault.dev       → Fargate: Next.js (port 3000)
  └─ auth.llmvault.dev  → Fargate: ZITADEL (port 8080)

RDS Postgres (private subnet, encrypted)
ElastiCache Redis (private subnet, encrypted)
KMS key (envelope encryption for API credentials)
```

**Staging (EC2):**
```
Cloudflare DNS
  ├─ dev.llmvault.dev         → A → Elastic IP
  ├─ api.dev.llmvault.dev     → A → Elastic IP
  ├─ auth.dev.llmvault.dev    → A → Elastic IP
  └─ connect.dev.llmvault.dev → CNAME → CloudFront

EC2 t4g.small
  └─ Docker Compose: Postgres, Redis, ZITADEL, API, Web
  └─ Caddy: TLS termination + domain routing (Let's Encrypt)
```

---

## Prerequisites

- **AWS CLI** configured with credentials (`aws configure`)
- **Node.js 22+** and **npm**
- **Docker** (for building images)
- **Cloudflare** account managing `llmvault.dev`
- **AWS account** with permissions for: ECS, EC2, RDS, ElastiCache, KMS, ACM, CloudFront, S3, SSM, IAM, VPC, ECR

## Quick Start

```bash
cd infra
npm install
```

---

## Step 1: Configure

Edit `lib/config.ts` with your AWS account ID:

```typescript
account: '123456789012',  // your AWS account ID
region: 'us-east-1',
```

Or set environment variables:
```bash
export CDK_DEFAULT_ACCOUNT=123456789012
export CDK_DEFAULT_REGION=us-east-1
```

## Step 2: Bootstrap CDK

First-time only. Sets up the CDK toolkit stack in your AWS account.

```bash
npx cdk bootstrap aws://123456789012/us-east-1
```

## Step 3: Deploy shared resources (ECR)

```bash
npx cdk deploy LLMVault-Shared
```

This creates the ECR repositories for API and Web images.

## Step 4: Build and push Docker images

You need images in ECR before deploying the compute stack.

```bash
# From the repo root (not infra/)
cd ..

# Login to ECR
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin 123456789012.dkr.ecr.us-east-1.amazonaws.com

# Build and push the Go API image
docker build -f docker/Dockerfile -t 123456789012.dkr.ecr.us-east-1.amazonaws.com/llmvault/api:latest .
docker push 123456789012.dkr.ecr.us-east-1.amazonaws.com/llmvault/api:latest

# Build and push the Next.js web image
docker build -f docker/Dockerfile.web \
  --build-arg NEXT_PUBLIC_API_URL=https://api.llmvault.dev \
  --build-arg NEXT_PUBLIC_ZITADEL_ISSUER=https://auth.llmvault.dev \
  --build-arg NEXT_PUBLIC_CONNECT_URL=https://connect.llmvault.dev \
  -t 123456789012.dkr.ecr.us-east-1.amazonaws.com/llmvault/web:latest .
docker push 123456789012.dkr.ecr.us-east-1.amazonaws.com/llmvault/web:latest

cd infra
```

## Step 5: Deploy networking + security

```bash
npx cdk deploy LLMVault-prod-Network LLMVault-prod-Security
```

## Step 6: Create SSM secrets

All secrets are stored in AWS SSM Parameter Store as SecureString.
Create them **before** deploying the data and compute stacks.

```bash
# Generate random values
JWT_KEY=$(openssl rand -base64 32)
REDIS_PASS=$(openssl rand -base64 24)
ZITADEL_MASTER=$(openssl rand -base64 24 | head -c 32)
ZITADEL_DB_PASS=$(openssl rand -base64 24)

# Store in SSM
aws ssm put-parameter --name "/llmvault/prod/jwt-signing-key" --value "$JWT_KEY" --type SecureString
aws ssm put-parameter --name "/llmvault/prod/redis-password" --value "$REDIS_PASS" --type SecureString
aws ssm put-parameter --name "/llmvault/prod/zitadel-masterkey" --value "$ZITADEL_MASTER" --type SecureString
aws ssm put-parameter --name "/llmvault/prod/zitadel-db-password" --value "$ZITADEL_DB_PASS" --type SecureString

# Placeholders for ZITADEL credentials (populated after ZITADEL starts)
aws ssm put-parameter --name "/llmvault/prod/zitadel-client-id" --value "placeholder" --type SecureString
aws ssm put-parameter --name "/llmvault/prod/zitadel-client-secret" --value "placeholder" --type SecureString
aws ssm put-parameter --name "/llmvault/prod/zitadel-admin-pat" --value "placeholder" --type SecureString
aws ssm put-parameter --name "/llmvault/prod/zitadel-project-id" --value "placeholder" --type SecureString
aws ssm put-parameter --name "/llmvault/prod/zitadel-dashboard-client-id" --value "placeholder" --type SecureString
```

## Step 7: Deploy data stores (production only)

```bash
npx cdk deploy LLMVault-prod-Data
```

This creates:
- RDS Postgres (encrypted, auto-generated password in Secrets Manager)
- ElastiCache Redis (encrypted, auth token from SSM)
- A Lambda that automatically constructs `DATABASE_URL` and stores it in SSM

## Step 8: Deploy compute + CDN

```bash
npx cdk deploy LLMVault-prod-Compute LLMVault-prod-Cdn
```

**Important: ACM certificate validation.** During this deploy, CloudFormation will
pause waiting for certificate validation. You need to:

1. Open the [AWS ACM console](https://console.aws.amazon.com/acm/)
2. Find the certificates in `PENDING_VALIDATION` status
3. Copy the CNAME name and value for each domain
4. Add these CNAME records in your **Cloudflare dashboard**
5. Wait ~2-5 minutes for validation to complete
6. CloudFormation resumes automatically

You will see CNAME records for each domain on the certificate (e.g., `_abc123.llmvault.dev`).
Each one gets a corresponding CNAME value. Add all of them in Cloudflare.

## Step 9: Configure Cloudflare DNS

After the deploy completes, the stack outputs the ALB DNS name and CloudFront domain.
Create these records in Cloudflare:

**Production (Fargate):**

| Type | Name | Target | Proxy |
|------|------|--------|-------|
| CNAME | `llmvault.dev` | `<ALB DNS name from output>` | DNS only (gray cloud) |
| CNAME | `api.llmvault.dev` | `<ALB DNS name from output>` | DNS only (gray cloud) |
| CNAME | `auth.llmvault.dev` | `<ALB DNS name from output>` | DNS only (gray cloud) |
| CNAME | `connect.llmvault.dev` | `<CloudFront domain from output>` | DNS only (gray cloud) |

> **Important:** Set Cloudflare proxy to **DNS only** (gray cloud icon). The ALB already
> handles TLS via ACM. If you enable Cloudflare proxy (orange cloud), you'll get
> double-TLS and potential certificate mismatch issues.

**Staging (EC2):**

| Type | Name | Target | Proxy |
|------|------|--------|-------|
| A | `dev.llmvault.dev` | `<Elastic IP from output>` | DNS only (gray cloud) |
| A | `api.dev.llmvault.dev` | `<Elastic IP from output>` | DNS only (gray cloud) |
| A | `auth.dev.llmvault.dev` | `<Elastic IP from output>` | DNS only (gray cloud) |
| CNAME | `connect.dev.llmvault.dev` | `<CloudFront domain from output>` | DNS only (gray cloud) |

## Step 10: Initialize ZITADEL

ZITADEL starts with `start-from-init` which creates its database automatically.
After it's healthy, you need to run the bootstrap script to create the project,
API app, and OIDC dashboard app.

**Fargate (production):**

```bash
# Find the ZITADEL task
TASK_ARN=$(aws ecs list-tasks --cluster llmvault-prod --service-name zitadel --query 'taskArns[0]' --output text)

# Extract the admin PAT from the running container
aws ecs execute-command \
  --cluster llmvault-prod \
  --task "$TASK_ARN" \
  --container zitadel \
  --interactive \
  --command "cat /tmp/admin.pat"
```

Copy the PAT, then run the init script against the ZITADEL endpoint:

```bash
export ZITADEL_URL=https://auth.llmvault.dev
export ZITADEL_EXTERNAL_URL=https://auth.llmvault.dev
export DASHBOARD_REDIRECT_URI=https://llmvault.dev/api/auth/callback/zitadel
export DASHBOARD_LOGOUT_URI=https://llmvault.dev
export ZITADEL_HOST_HEADER=auth.llmvault.dev
export ZITADEL_PAT_FILE=/dev/stdin

echo "<paste PAT here>" | bash docker/zitadel/init.sh
```

The script outputs the credentials. Store them in SSM:

```bash
aws ssm put-parameter --name "/llmvault/prod/zitadel-client-id" --value "<CLIENT_ID>" --type SecureString --overwrite
aws ssm put-parameter --name "/llmvault/prod/zitadel-client-secret" --value "<CLIENT_SECRET>" --type SecureString --overwrite
aws ssm put-parameter --name "/llmvault/prod/zitadel-admin-pat" --value "<PAT>" --type SecureString --overwrite
aws ssm put-parameter --name "/llmvault/prod/zitadel-project-id" --value "<PROJECT_ID>" --type SecureString --overwrite
aws ssm put-parameter --name "/llmvault/prod/zitadel-dashboard-client-id" --value "<DASHBOARD_CLIENT_ID>" --type SecureString --overwrite
```

Then force a new deployment so the API picks up the real credentials:

```bash
aws ecs update-service --cluster llmvault-prod --service api --force-new-deployment
```

---

## Deploy Staging

Repeat steps 5-10 but with `staging` stacks and `dev.llmvault.dev` domains:

```bash
npx cdk deploy LLMVault-staging-Network LLMVault-staging-Security

# Create SSM secrets for staging (same pattern as step 6 but with /llmvault/staging/ prefix)
# ...

npx cdk deploy LLMVault-staging-Compute LLMVault-staging-Cdn
```

The staging EC2 instance uses Caddy for TLS (Let's Encrypt). Make sure the
Cloudflare A records are created **before** the EC2 starts, so Caddy can
validate the domains.

---

## CI/CD (CodeBuild — recommended)

AWS CodeBuild builds Docker images on ARM64 natively (no QEMU emulation),
pushes to ECR with zero egress, and deploys automatically. Triggered by
GitHub webhook — no GitHub Actions minutes consumed for builds.

| Trigger | Action |
|---------|--------|
| Push to `main` | Build images, push to ECR |
| Push to `develop` | Build + deploy to staging |
| Tag `v*` | Build + deploy staging + deploy production |

### One-time setup: connect GitHub to CodeBuild

Create a GitHub personal access token (PAT) with `repo` and `admin:repo_hook`
scopes, then register it with CodeBuild:

```bash
aws codebuild import-source-credentials \
  --server-type GITHUB \
  --auth-type PERSONAL_ACCESS_TOKEN \
  --token ghp_xxxxxxxxxxxx
```

Then deploy the build stack:

```bash
npx cdk deploy LLMVault-Build
```

This creates the CodeBuild project and registers a webhook on the GitHub
repo. All subsequent pushes trigger builds automatically.

### Configuration

Edit `lib/config.ts` to change the GitHub owner/repo:

```typescript
export const github = {
  owner: 'useportal',
  repo: 'llmvault',
};
```

The buildspec lives at `buildspec.yml` in the repo root. It references
environment variables injected by CDK (ECR registry, domain names, bucket
names, etc.), so you rarely need to edit it directly.

### Costs

CodeBuild ARM `arm1.small` — $0.0034/min. A typical build takes ~8 minutes
(~$0.03 per build). First 100 minutes/month are free.

---

## CI/CD (GitHub Actions — alternative)

If you prefer GitHub Actions, the workflow at `.github/workflows/deploy.yml`
is a drop-in alternative. It uses GitHub OIDC for AWS authentication.

> **Note:** GitHub Actions builds ARM images via QEMU emulation, which is
> slower than native ARM on CodeBuild. Use CodeBuild if build speed matters.

### Required GitHub Secrets

| Secret | Value |
|--------|-------|
| `AWS_ACCOUNT_ID` | Your AWS account ID |
| `AWS_DEPLOY_ROLE_ARN` | IAM role ARN for GitHub OIDC (see below) |
| `NEXT_PUBLIC_API_URL` | `https://api.llmvault.dev` |
| `NEXT_PUBLIC_ZITADEL_ISSUER` | `https://auth.llmvault.dev` |
| `NEXT_PUBLIC_CONNECT_URL` | `https://connect.llmvault.dev` |
| `NEXT_PUBLIC_ZITADEL_CLIENT_ID` | ZITADEL dashboard client ID |

### GitHub OIDC Setup

Create an IAM OIDC identity provider for GitHub Actions (one-time):

```bash
aws iam create-open-id-connect-provider \
  --url https://token.actions.githubusercontent.com \
  --client-id-list sts.amazonaws.com \
  --thumbprint-list 6938fd4d98bab03faadb97b34396831e3780aea1

cat > trust-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {
      "Federated": "arn:aws:iam::YOUR_ACCOUNT_ID:oidc-provider/token.actions.githubusercontent.com"
    },
    "Action": "sts:AssumeRoleWithWebIdentity",
    "Condition": {
      "StringEquals": {
        "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
      },
      "StringLike": {
        "token.actions.githubusercontent.com:sub": "repo:YOUR_ORG/YOUR_REPO:*"
      }
    }
  }]
}
EOF

aws iam create-role --role-name llmvault-github-deploy --assume-role-policy-document file://trust-policy.json
```

Attach policies for ECR push, ECS deploy, SSM, S3, and CloudFront invalidation.

---

## Scaling

All scaling is done by editing `lib/config.ts` and running `cdk deploy`.

### Enable HA (multi-AZ)

```typescript
// lib/config.ts — production
rdsMultiAz: true,       // +$12/mo — database failover across AZs
tasksPerService: 2,      // +$29/mo — 2 Fargate tasks per service across AZs
```

```bash
npx cdk deploy LLMVault-prod-Data LLMVault-prod-Compute
```

### Upgrade staging to Fargate

```typescript
// lib/config.ts — staging
phase: 'fargate',
```

Then create the SSM secrets for staging's RDS and deploy:

```bash
npx cdk deploy LLMVault-staging-Data LLMVault-staging-Compute
```

### Increase instance sizes

```typescript
fargate: {
  api: { cpu: 512, memory: 1024 },     // double the API resources
},
rdsInstanceClass: 'db.t4g.small',        // bigger database
redisNodeType: 'cache.t4g.small',        // bigger cache
```

---

## CDK Stacks Reference

| Stack | Purpose |
|-------|---------|
| `LLMVault-Shared` | ECR repositories (deploy once) |
| `LLMVault-Build` | CodeBuild project + GitHub webhook (deploy once) |
| `LLMVault-{env}-Network` | VPC, subnets, security groups |
| `LLMVault-{env}-Security` | KMS key, IAM roles |
| `LLMVault-{env}-Data` | RDS + ElastiCache (Fargate mode only) |
| `LLMVault-{env}-Compute` | ALB + Fargate services (or EC2) |
| `LLMVault-{env}-Cdn` | S3 + CloudFront for Connect SPA |

## SSM Parameters Reference

All secrets stored under `/llmvault/{env}/`:

| Parameter | Source |
|-----------|--------|
| `database-url` | Auto-generated by CDK Lambda (from RDS credentials) |
| `redis-password` | You generate during setup |
| `jwt-signing-key` | You generate during setup |
| `zitadel-masterkey` | You generate during setup (exactly 32 chars) |
| `zitadel-db-password` | You generate during setup |
| `zitadel-client-id` | From ZITADEL init script output |
| `zitadel-client-secret` | From ZITADEL init script output |
| `zitadel-admin-pat` | From ZITADEL bootstrap |
| `zitadel-project-id` | From ZITADEL init script output |
| `zitadel-dashboard-client-id` | From ZITADEL init script output |
