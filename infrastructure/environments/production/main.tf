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
