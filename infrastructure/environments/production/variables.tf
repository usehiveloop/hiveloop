# =============================================================================
# Variables - Production Environment
# =============================================================================

variable "aws_region" {
  description = "AWS region for resources"
  type        = string
  default     = "us-east-2"
}

variable "domain_name" {
  description = "Primary domain for the application"
  type        = string
  default     = "llmvault.dev"
}

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "production"
}

variable "project_name" {
  description = "Project name for resource naming"
  type        = string
  default     = "llmvault"
}
