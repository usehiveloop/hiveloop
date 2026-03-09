# =============================================================================
# Terraform Backend Configuration
# 
# For first-time setup, comment out the S3 backend and run locally,
# or run the bootstrap.sh script first to create the S3 bucket.
# =============================================================================

# Option 1: S3 Backend (Production)
# Run ./bootstrap.sh first to create the bucket
terraform {
  backend "s3" {
    bucket       = "llmvault-terraform-state"
    key          = "production/terraform.tfstate"
    region       = "us-east-2"
    encrypt      = true
    use_lockfile = true  # New parameter for Terraform 1.13+
    # Note: DynamoDB table no longer needed with use_lockfile
  }
}

# Option 2: Local Backend (Initial testing)
# Uncomment below and comment out the S3 backend above for local testing
# terraform {
#   backend "local" {
#     path = "terraform.tfstate"
#   }
# }
