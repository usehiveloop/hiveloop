# =============================================================================
# Outputs - Production Environment (Phase 0)
# =============================================================================

output "vpc_id" {
  description = "VPC ID"
  value       = module.vpc.vpc_id
}

output "vpc_cidr" {
  description = "VPC CIDR block"
  value       = module.vpc.vpc_cidr
}

output "public_subnet_ids" {
  description = "Public subnet IDs"
  value       = module.vpc.public_subnet_ids
}

output "private_subnet_ids" {
  description = "Private subnet IDs"
  value       = module.vpc.private_subnet_ids
}

output "ecs_cluster_name" {
  description = "ECS Cluster name"
  value       = module.ecs_cluster.cluster_name
}

output "ecs_cluster_arn" {
  description = "ECS Cluster ARN"
  value       = module.ecs_cluster.cluster_arn
}

output "ecr_repository_urls" {
  description = "ECR Repository URLs"
  value       = module.ecr.repository_urls
}

output "ecs_task_execution_role_arn" {
  description = "ECS Task Execution Role ARN"
  value       = module.iam.ecs_task_execution_role_arn
}

output "ecs_task_role_arn" {
  description = "ECS Task Role ARN"
  value       = module.iam.ecs_task_role_arn
}

# =============================================================================
# Phase 1: Database Outputs
# =============================================================================

output "rds_endpoint" {
  description = "RDS PostgreSQL endpoint"
  value       = module.rds.endpoint
}

output "rds_port" {
  description = "RDS PostgreSQL port"
  value       = module.rds.port
}

output "rds_db_name" {
  description = "RDS database name"
  value       = module.rds.db_name
}

output "rds_username" {
  description = "RDS master username"
  value       = module.rds.username
}

output "rds_secrets_manager_arn" {
  description = "Secrets Manager ARN for database credentials"
  value       = module.rds.secrets_manager_arn
}

output "rds_security_group_id" {
  description = "RDS Security Group ID"
  value       = module.rds.security_group_id
}

# =============================================================================
# Phase 2: Cache Outputs
# =============================================================================

output "elasticache_endpoint" {
  description = "ElastiCache Redis endpoint"
  value       = module.elasticache.endpoint
}

output "elasticache_port" {
  description = "ElastiCache Redis port"
  value       = module.elasticache.port
}

output "elasticache_redis_url" {
  description = "Redis connection URL"
  value       = module.elasticache.redis_url
}

output "elasticache_security_group_id" {
  description = "ElastiCache Security Group ID"
  value       = module.elasticache.security_group_id
}
