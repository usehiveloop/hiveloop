# =============================================================================
# ECS Cluster Module Variables
# =============================================================================

variable "name" {
  description = "ECS Cluster name"
  type        = string
}

variable "enable_container_insights" {
  description = "Enable CloudWatch Container Insights"
  type        = bool
  default     = true
}

variable "log_retention_days" {
  description = "CloudWatch log retention in days"
  type        = number
  default     = 7  # Short retention to minimize cost
}

variable "services" {
  description = "List of service names (creates log groups)"
  type        = list(string)
  default     = ["api", "web", "zitadel"]
}
