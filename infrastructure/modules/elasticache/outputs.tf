# =============================================================================
# ElastiCache Module Outputs
# =============================================================================

output "cluster_id" {
  description = "ElastiCache cluster ID"
  value       = aws_elasticache_cluster.main.id
}

output "cluster_arn" {
  description = "ElastiCache cluster ARN"
  value       = aws_elasticache_cluster.main.arn
}

output "endpoint" {
  description = "ElastiCache Redis endpoint (hostname only)"
  value       = aws_elasticache_cluster.main.cache_nodes[0].address
}

output "port" {
  description = "ElastiCache Redis port"
  value       = aws_elasticache_cluster.main.port
}

output "redis_url" {
  description = "Redis connection URL"
  value       = "redis://${aws_elasticache_cluster.main.cache_nodes[0].address}:${aws_elasticache_cluster.main.port}/0"
}

output "security_group_id" {
  description = "ElastiCache Security Group ID"
  value       = aws_security_group.elasticache.id
}

output "subnet_group_name" {
  description = "ElastiCache subnet group name"
  value       = aws_elasticache_subnet_group.main.name
}

output "parameter_group_name" {
  description = "ElastiCache parameter group name"
  value       = aws_elasticache_parameter_group.main.name
}
