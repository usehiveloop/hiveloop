# Connect App

Static Vite React app deployed to AWS S3 + CloudFront with custom domain.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  User → connect.dev.ziraloop.com / connect.ziraloop.com     │
│       ↓                                                     │
│  Cloudflare DNS (CNAME → CloudFront)                        │
│       ↓                                                     │
│  CloudFront (HTTPS with ACM cert in us-east-1)              │
│       ↓                                                     │
│  S3 Bucket (static files)                                   │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### Local Development

```bash
# Install dependencies
npm install

# Start dev server
npm run dev
```

### Deployment

#### Automatic (GitHub Actions)

| Environment | Trigger | URL |
|-------------|---------|-----|
| Development | Push to `main` | https://connect.dev.ziraloop.com |
| Production | GitHub Release published | https://connect.ziraloop.com |

#### Manual

```bash
# Deploy to development
npm run deploy:dev

# Deploy to production
npm run deploy:prod
```

## First-Time Setup (New Environment)

### 1. Deploy Infrastructure

```bash
cd infrastructure/cdk

# Development
npx cdk deploy ZiraLoop-Dev-Connect

# Production  
npx cdk deploy ZiraLoop-Prod-Connect
```

**Save these outputs:**
- `CloudFrontDomain` (e.g., `d2jeixo95enek0.cloudfront.net`)
- `DistributionId` (e.g., `E1SRM0XYQG4Y06`)
- `S3BucketName` (e.g., `ziraloop-dev-connect-assets`)

### 2. Create & Validate Certificate

CloudFront requires ACM certificates in `us-east-1`.

```bash
cd infrastructure/cdk

# Request certificate
./scripts/setup-cert.sh dev    # or prod
```

**Output will show DNS validation CNAME:**
```
Type:  CNAME
Name:  _abc123.connect.dev.ziraloop.com
Value: _xyz789.acm-validations.aws
```

Add this CNAME to **Cloudflare DNS**, then wait for validation (5-30 min).

### 3. Update CloudFront with Certificate

After certificate shows status `ISSUED`:

```bash
# Option A: Update via CLI
./scripts/update-cloudfront.sh dev <CERTIFICATE_ARN>

# Option B: Update config and redeploy
# Edit infrastructure/cdk/config/certificates.json
npx cdk deploy ZiraLoop-Dev-Connect
```

### 4. Add DNS Record

In **Cloudflare DNS**, add:

**Development:**
| Type | Name | Target | Proxy Status |
|------|------|--------|--------------|
| CNAME | `connect.dev` | `d2jeixo95enek0.cloudfront.net` | DNS only (gray) |

**Production:**
| Type | Name | Target | Proxy Status |
|------|------|--------|--------------|
| CNAME | `connect` | `[CloudFront domain]` | DNS only (gray) |

### 5. Deploy App

```bash
cd apps/connect

# Development
npm run deploy:dev

# Production
npm run deploy:prod
```

## Available Scripts

| Command | Description |
|---------|-------------|
| `npm run dev` | Start development server |
| `npm run build` | Build for development |
| `npm run build:prod` | Build for production |
| `npm run preview` | Preview production build locally |
| `npm run deploy:dev` | Deploy to connect.dev.ziraloop.com |
| `npm run deploy:prod` | Deploy to connect.ziraloop.com |

## Project Structure

```
apps/connect/
├── scripts/
│   └── deploy.sh              # Deployment script
├── src/                       # React source code
├── public/                    # Static assets
├── dist/                      # Build output (gitignored)
├── index.html                 # Entry HTML
├── vite.config.ts             # Vite configuration
├── package.json               # Dependencies & scripts
└── README.md                  # This file
```

## GitHub Actions Setup

see `.github/workflows/readme.md` for detailed setup instructions.

quick setup:
1. create iam role for github actions (oidc) in aws
2. add `aws_role_arn_dev` and `aws_role_arn_prod` secrets to github
3. push to `main` triggers dev deployment
4. create a release triggers prod deployment

### Certificate ARNs

`infrastructure/cdk/config/certificates.json`:
```json
{
  "dev": {
    "connect": "arn:aws:acm:us-east-1:944765969440:certificate/..."
  },
  "prod": {
    "connect": null
  }
}
```

Add production certificate ARN here after creating it.

## Environment Checklist

### Development (connect.dev.ziraloop.com)

- [ ] CDK stack deployed
- [ ] Certificate created in us-east-1
- [ ] DNS validation CNAME added to Cloudflare
- [ ] Certificate status is `ISSUED`
- [ ] CloudFront updated with certificate
- [ ] App DNS CNAME added to Cloudflare
- [ ] App deployed successfully

### Production (connect.ziraloop.com)

- [ ] CDK stack deployed
- [ ] Certificate created in us-east-1
- [ ] DNS validation CNAME added to Cloudflare
- [ ] Certificate status is `ISSUED`
- [ ] CloudFront updated with certificate
- [ ] App DNS CNAME added to Cloudflare
- [ ] App deployed successfully

## Useful Commands

```bash
# Check certificate status
aws acm describe-certificate \
  --certificate-arn <ARN> \
  --region us-east-1 \
  --query 'Certificate.Status'

# List all certificates
aws acm list-certificates --region us-east-1

# Check CloudFront distribution
aws cloudfront get-distribution --id <DISTRIBUTION_ID>

# Wait for CloudFront deployment
aws cloudfront wait distribution-deployed --id <DISTRIBUTION_ID>

# Invalidate cache manually
aws cloudfront create-invalidation \
  --distribution-id <DISTRIBUTION_ID> \
  --paths "/*"
```

## Troubleshooting

### 403 Forbidden from CloudFront

- Check S3 bucket has files: `aws s3 ls s3://bucket-name/`
- Verify CloudFront Origin Access Identity has S3 read permission

### Certificate not found

- Ensure certificate is in `us-east-1` region
- CloudFront only sees us-east-1 certificates

### Custom domain not working

- Check DNS CNAME is correct in Cloudflare
- Verify certificate is validated (status = `ISSUED`)
- Check CloudFront distribution includes domain in aliases
- Ensure CloudFront deployment is complete (not `InProgress`)

### Deployment fails

- Check AWS credentials are configured
- Ensure stack is deployed: `aws cloudformation describe-stacks --stack-name ZiraLoop-Dev-Connect`

## Technologies

- [Vite](https://vitejs.dev/) - Build tool
- [React](https://react.dev/) - UI library
- [Tailwind CSS](https://tailwindcss.com/) - Styling
- [AWS S3](https://aws.amazon.com/s3/) - Static hosting
- [AWS CloudFront](https://aws.amazon.com/cloudfront/) - CDN
- [AWS Certificate Manager](https://aws.amazon.com/certificate-manager/) - SSL certificates
