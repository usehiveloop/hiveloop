# =============================================================================
# ECR Module Outputs
# =============================================================================

output "repository_arns" {
  description = "ECR Repository ARNs"
  value       = { for name, repo in aws_ecr_repository.main : name => repo.arn }
}

output "repository_urls" {
  description = "ECR Repository URLs"
  value       = { for name, repo in aws_ecr_repository.main : name => repo.repository_url }
}

output "repository_names" {
  description = "ECR Repository names"
  value       = { for name, repo in aws_ecr_repository.main : name => repo.name }
}
