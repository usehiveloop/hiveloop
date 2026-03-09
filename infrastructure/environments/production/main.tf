# =============================================================================
# LLMVault Production - Phase 0: Foundation
# Region: us-east-2
# Domain: llmvault.dev
# =============================================================================

terraform {
  required_version = ">= 1.5.0"
  
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "llmvault"
      Environment = "production"
      ManagedBy   = "terraform"
    }
  }
}

# =============================================================================
# Phase 0: Foundation
# =============================================================================

module "vpc" {
  source = "../../modules/vpc"

  name               = "llmvault-prod"
  vpc_cidr           = "10.0.0.0/16"
  availability_zones = ["us-east-2a", "us-east-2b"]
  
  public_subnet_cidrs  = ["10.0.1.0/24", "10.0.2.0/24"]
  private_subnet_cidrs = ["10.0.10.0/24", "10.0.11.0/24"]
  
  enable_nat_gateway = false  # Phase 3
  enable_vpn_gateway = false
}

module "ecr" {
  source = "../../modules/ecr"

  repositories = ["api", "web", "zitadel", "connect"]
  
  image_tag_mutability = "IMMUTABLE"
  scan_on_push         = true
}

module "iam" {
  source = "../../modules/iam"

  name_prefix = "llmvault-prod"
}

module "ecs_cluster" {
  source = "../../modules/ecs-cluster"

  name = "llmvault-prod"
  
  services = ["api", "web", "zitadel", "connect"]
  
  enable_container_insights = true
}

# =============================================================================
# Phase 1: Database Layer
# =============================================================================

module "rds" {
  source = "../../modules/rds"

  name       = "llmvault-prod"
  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnet_ids
  
  # Allow ECS tasks to connect
  allowed_security_group_ids = [module.vpc.vpc_endpoint_security_group_id]
  
  # Database configuration
  db_name             = "llmvault"
  db_username         = "llmvault"
  engine_version      = "17.4"
  instance_class      = "db.t4g.micro"
  allocated_storage   = 20
  max_allocated_storage = 100
  
  # Backup settings
  backup_retention_days = 7
  backup_window         = "03:00-04:00"
  maintenance_window    = "Mon:04:00-Mon:05:00"
  
  # Protection (disable for easier iteration)
  deletion_protection = false
  skip_final_snapshot = true
}

# =============================================================================
# Phase 2: Cache Layer
# =============================================================================

module "elasticache" {
  source = "../../modules/elasticache"

  name       = "llmvault-prod"
  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnet_ids
  
  # Allow ECS tasks to connect
  allowed_security_group_ids = [module.vpc.vpc_endpoint_security_group_id]
  
  # Cache configuration
  engine_version        = "7.1"
  node_type             = "cache.t4g.micro"
  snapshot_retention_days = 1
  snapshot_window       = "05:00-06:00"
  maintenance_window    = "sun:06:00-sun:07:00"
}
