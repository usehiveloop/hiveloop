# =============================================================================
# ElastiCache Module Variables
# =============================================================================

variable "name" {
  description = "Name prefix for ElastiCache resources"
  type        = string
}

variable "vpc_id" {
  description = "VPC ID for security group"
  type        = string
}

variable "subnet_ids" {
  description = "List of private subnet IDs for cache subnet group"
  type        = list(string)
}

variable "allowed_security_group_ids" {
  description = "Security group IDs allowed to connect to ElastiCache"
  type        = list(string)
  default     = []
}

variable "engine_version" {
  description = "Redis engine version"
  type        = string
  default     = "7.1"
}

variable "node_type" {
  description = "ElastiCache node type"
  type        = string
  default     = "cache.t4g.micro"  # 0.5 vCPU, 0.5GB RAM, Graviton2
}

variable "snapshot_retention_days" {
  description = "Number of days to retain snapshots"
  type        = number
  default     = 1  # Minimal retention for cost
}

variable "snapshot_window" {
  description = "Preferred snapshot window (UTC)"
  type        = string
  default     = "05:00-06:00"  # 5-6 AM UTC
}

variable "maintenance_window" {
  description = "Preferred maintenance window (UTC)"
  type        = string
  default     = "sun:06:00-sun:07:00"  # Sunday 6-7 AM UTC
}
