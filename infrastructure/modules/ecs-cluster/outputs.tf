# =============================================================================
# ECS Cluster Module Outputs
# =============================================================================

output "cluster_name" {
  description = "ECS Cluster name"
  value       = aws_ecs_cluster.main.name
}

output "cluster_arn" {
  description = "ECS Cluster ARN"
  value       = aws_ecs_cluster.main.arn
}

output "cluster_id" {
  description = "ECS Cluster ID"
  value       = aws_ecs_cluster.main.id
}

output "log_group_name" {
  description = "Main CloudWatch Log Group name"
  value       = aws_cloudwatch_log_group.ecs.name
}

output "service_log_groups" {
  description = "Service-specific CloudWatch Log Group names"
  value       = { for name, log_group in aws_cloudwatch_log_group.services : name => log_group.name }
}
