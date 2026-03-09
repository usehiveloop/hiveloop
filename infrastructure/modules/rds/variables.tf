# =============================================================================
# RDS Module Variables
# =============================================================================

variable "name" {
  description = "Name prefix for RDS resources"
  type        = string
}

variable "vpc_id" {
  description = "VPC ID for security group"
  type        = string
}

variable "subnet_ids" {
  description = "List of private subnet IDs for DB subnet group"
  type        = list(string)
}

variable "allowed_security_group_ids" {
  description = "Security group IDs allowed to connect to RDS"
  type        = list(string)
  default     = []
}

variable "db_name" {
  description = "Database name (defaults to var.name with dashes replaced by underscores)"
  type        = string
  default     = ""
}

variable "db_username" {
  description = "Database master username"
  type        = string
  default     = "llmvault"
}

variable "engine_version" {
  description = "PostgreSQL engine version"
  type        = string
  default     = "17.4"
}

variable "instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.t4g.micro"  # 2 vCPU, 1GB RAM, Graviton2
}

variable "allocated_storage" {
  description = "Initial storage allocation in GB"
  type        = number
  default     = 20
}

variable "max_allocated_storage" {
  description = "Maximum storage for autoscaling in GB"
  type        = number
  default     = 100
}

variable "backup_retention_days" {
  description = "Number of days to retain backups"
  type        = number
  default     = 7
}

variable "backup_window" {
  description = "Preferred backup window (UTC)"
  type        = string
  default     = "03:00-04:00"  # 3-4 AM UTC
}

variable "maintenance_window" {
  description = "Preferred maintenance window (UTC)"
  type        = string
  default     = "Mon:04:00-Mon:05:00"  # Monday 4-5 AM UTC
}

variable "deletion_protection" {
  description = "Enable deletion protection"
  type        = bool
  default     = false  # Set to true for production
}

variable "skip_final_snapshot" {
  description = "Skip final snapshot on deletion"
  type        = bool
  default     = true  # Set to false for production
}
