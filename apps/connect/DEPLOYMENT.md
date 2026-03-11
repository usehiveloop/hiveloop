# Connect App Deployment Guide

## Overview

Static Vite React app deployed to AWS S3 + CloudFront with custom domain.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  User → connect.dev.llmvault.dev                            │
│       ↓                                                     │
│  Cloudflare DNS (CNAME → CloudFront)                        │
│       ↓                                                     │
│  CloudFront (HTTPS with ACM cert)                           │
│       ↓                                                     │
│  S3 Bucket (static files)                                   │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

- AWS CLI configured
- Access to Cloudflare DNS
- Certificate must be in `us-east-1` (CloudFront requirement)

## First-Time Setup (Per Environment)

### Step 1: Deploy Infrastructure (CDK)

```bash
cd infrastructure/cdk

# Development
npx cdk deploy LlmVault-Dev-Connect

# Production
npx cdk deploy LlmVault-Prod-Connect
```

**Outputs to save:**
- `CloudFrontDomain` (e.g., `d2jeixo95enek0.cloudfront.net`)
- `DistributionId` (e.g., `E1SRM0XYQG4Y06`)
- `S3BucketName` (e.g., `llmvault-dev-connect-assets`)

### Step 2: Create & Validate Certificate

```bash
cd infrastructure/cdk

# Development
./scripts/setup-cert.sh dev

# Production
./scripts/setup-cert.sh prod
```

This will:
1. Request ACM certificate in `us-east-1`
2. Output DNS validation CNAME record
3. Wait for validation (optional)

**Add the validation CNAME to Cloudflare**, then proceed.

### Step 3: Link Certificate to CloudFront

After certificate is validated (status = `ISSUED`):

```bash
# Get the certificate ARN
aws acm list-certificates --region us-east-1 \
  --query "CertificateSummaryList[?DomainName=='connect.dev.llmvault.dev'].CertificateArn" \
  --output text

# Update CloudFront distribution
./scripts/update-cloudfront.sh dev <CERTIFICATE_ARN>
```

### Step 4: Add DNS Record

In Cloudflare, add CNAME:

| Type | Name | Target | Proxy Status |
|------|------|--------|--------------|
| CNAME | `connect.dev` | `d2jeixo95enek0.cloudfront.net` | DNS only (gray) |

For production:
| CNAME | `connect` | `[CloudFront domain]` | DNS only (gray) |

## Regular Deployments

After setup is complete, deploy app updates with:

```bash
cd apps/connect

# Development
npm run deploy:dev

# Production
npm run deploy:prod
```

This will:
1. Build the Vite app
2. Sync to S3
3. Invalidate CloudFront cache

## Quick Reference

### Commands

| Task | Command |
|------|---------|
| Deploy infra (dev) | `cd infrastructure/cdk && npx cdk deploy LlmVault-Dev-Connect` |
| Deploy infra (prod) | `cd infrastructure/cdk && npx cdk deploy LlmVault-Prod-Connect` |
| Setup cert (dev) | `cd infrastructure/cdk && ./scripts/setup-cert.sh dev` |
| Setup cert (prod) | `cd infrastructure/cdk && ./scripts/setup-cert.sh prod` |
| Deploy app (dev) | `cd apps/connect && npm run deploy:dev` |
| Deploy app (prod) | `cd apps/connect && npm run deploy:prod` |

### Resources Created

| Environment | S3 Bucket | CloudFront Domain | Custom Domain |
|-------------|-----------|-------------------|---------------|
| dev | `llmvault-dev-connect-assets` | Auto-generated | `connect.dev.llmvault.dev` |
| prod | `llmvault-prod-connect-assets` | Auto-generated | `connect.llmvault.dev` |

## Troubleshooting

### Certificate not found
CloudFront only sees certificates in `us-east-1`. Verify:
```bash
aws acm describe-certificate \
  --certificate-arn <ARN> \
  --region us-east-1
```

### CloudFront returns 403
- Check S3 bucket policy allows CloudFront OAI
- Verify files exist in S3: `aws s3 ls s3://bucket-name/`

### Custom domain not working
- Check DNS CNAME is correct
- Verify certificate is validated and ISSUED
- Check CloudFront distribution includes the domain in aliases

## Production Checklist

Before deploying to production:

- [ ] Certificate created and validated in us-east-1
- [ ] CloudFront distribution updated with certificate
- [ ] DNS CNAME record added to Cloudflare
- [ ] App builds successfully (`npm run build:prod`)
- [ ] Test deployment to verify
- [ ] Set up monitoring/alarms (optional)
