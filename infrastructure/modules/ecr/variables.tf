# =============================================================================
# ECR Module Variables
# =============================================================================

variable "name_prefix" {
  description = "Prefix for repository names"
  type        = string
  default     = "llmvault"
}

variable "repositories" {
  description = "List of repository names to create"
  type        = list(string)
}

variable "image_tag_mutability" {
  description = "Image tag mutability (MUTABLE or IMMUTABLE)"
  type        = string
  default     = "IMMUTABLE"
}

variable "scan_on_push" {
  description = "Enable image scanning on push"
  type        = bool
  default     = true
}

variable "keep_last_n_images" {
  description = "Number of images to retain"
  type        = number
  default     = 10
}

variable "force_delete" {
  description = "Force delete repository (allows terraform destroy)"
  type        = bool
  default     = true
}
