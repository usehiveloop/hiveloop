# LLMVault Infrastructure

Terraform infrastructure for LLMVault production deployment on AWS.

## Architecture Overview

| Component | Technology | Cost/Month |
|-----------|-----------|------------|
| Compute | ECS Fargate (ARM64) | ~$30 |
| Database | RDS PostgreSQL (db.t4g.micro) | ~$12 |
| Cache | ElastiCache Redis (cache.t4g.micro) | ~$12 |
| Networking | ALB + NAT Gateway | ~$55 |
| **Total** | | **~$110** |

## Project Structure

```
infrastructure/
├── bootstrap.sh              # One-time backend setup
├── modules/
│   ├── vpc/                  # Network foundation
│   ├── ecr/                  # Container registries
│   ├── iam/                  # IAM roles
│   ├── ecs-cluster/          # ECS cluster
│   ├── rds/                  # Database (Phase 1)
│   ├── elasticache/          # Cache (Phase 2)
│   ├── networking/           # ALB, NAT, VPC Endpoints (Phase 3)
│   └── ecs-service/          # Reusable ECS service (Phase 4-6)
└── environments/
    └── production/           # Production environment
```

## Prerequisites

- AWS CLI configured (`aws configure`)
- Terraform >= 1.5.0
- Domain `llmvault.dev` registered in Route 53

## Deployment Phases

### Phase 0: Foundation (Current)

Creates the base infrastructure with **$0** running cost:
- VPC with public/private subnets (2 AZs)
- ECS Cluster
- ECR Repositories (api, web, zitadel)
- IAM Roles

**Deploy:**
```bash
# 1. Bootstrap backend (one-time)
./bootstrap.sh

# 2. Initialize Terraform
cd environments/production
terraform init

# 3. Plan and apply
terraform plan
terraform apply
```

**Verify:**
```bash
# Check VPC
aws ec2 describe-vpcs --vpc-ids $(terraform output -raw vpc_id) --region us-east-2

# Check ECS Cluster
aws ecs describe-clusters --clusters llmvault-prod --region us-east-2

# Check ECR Repositories
aws ecr describe-repositories --region us-east-2
```

### Phase 1: Database (Next)

Adds RDS PostgreSQL (**~$12/month**).

### Phase 2: Cache

Adds ElastiCache Redis (**~$12/month**).

### Phase 3: Networking

Adds ALB, NAT Gateway, VPC Endpoints (**~$55/month**).

### Phase 4-6: Services

Deploys ZITADEL, API, Web services (**~$30/month**).

## Domains

| Service | Domain |
|---------|--------|
| Web Dashboard | `llmvault.dev` |
| Auth (ZITADEL) | `auth.llmvault.dev` |
| API | `api.llmvault.dev` |
| Connect UI | `connect.llmvault.dev` |

## Security

- All services run in private subnets
- Only ALB is exposed to internet
- Secrets managed via AWS Secrets Manager
- KMS encryption for sensitive data
- IAM roles follow least privilege

## Cost Optimization

- **ARM64 (Graviton)** processors: 20% cheaper
- **Fargate** vs EC2: Better isolation, pay per use
- **Reserved capacity**: Consider 1-year Savings Plan for ~20% discount
- **Lifecycle policies**: ECR keeps only last 10 images
- **Log retention**: 7 days to minimize CloudWatch costs

## Rollback

Each phase can be destroyed independently:
```bash
# Destroy specific resource
cd environments/production
terraform destroy -target=module.ecs_cluster

# Destroy entire environment (DANGER)
terraform destroy
```

## Monitoring

After Phase 7, CloudWatch alarms monitor:
- RDS CPU > 80%
- ECS task health
- ALB 5xx errors
- Monthly budget ($150 threshold)
