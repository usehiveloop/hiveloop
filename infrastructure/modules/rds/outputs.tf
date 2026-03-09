# =============================================================================
# RDS Module Outputs
# =============================================================================

output "db_instance_id" {
  description = "RDS instance ID"
  value       = aws_db_instance.main.id
}

output "db_instance_arn" {
  description = "RDS instance ARN"
  value       = aws_db_instance.main.arn
}

output "endpoint" {
  description = "RDS instance endpoint (hostname only)"
  value       = aws_db_instance.main.address
}

output "port" {
  description = "RDS instance port"
  value       = aws_db_instance.main.port
}

output "db_name" {
  description = "Database name"
  value       = aws_db_instance.main.db_name
}

output "username" {
  description = "Database master username"
  value       = aws_db_instance.main.username
}

output "security_group_id" {
  description = "Security group ID for RDS"
  value       = aws_security_group.rds.id
}

output "subnet_group_id" {
  description = "DB subnet group ID"
  value       = aws_db_subnet_group.main.id
}

output "secrets_manager_arn" {
  description = "Secrets Manager ARN for database credentials"
  value       = aws_secretsmanager_secret.db_credentials.arn
}

output "secrets_manager_name" {
  description = "Secrets Manager secret name"
  value       = aws_secretsmanager_secret.db_credentials.name
}
