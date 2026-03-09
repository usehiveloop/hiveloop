#!/bin/bash
# =============================================================================
# Bootstrap Script for LLMVault Terraform Backend
# Creates S3 bucket for state management
# Run once per AWS account before first terraform apply
# 
# Note: Terraform 1.13+ uses native S3 locking (use_lockfile), 
# DynamoDB table is no longer required.
# =============================================================================

set -e

REGION="us-east-2"
BUCKET_NAME="llmvault-terraform-state"

echo "=== Bootstrapping LLMVault Terraform Backend ==="
echo "Region: $REGION"
echo ""

# Check if AWS CLI is configured
echo "Checking AWS credentials..."
aws sts get-caller-identity > /dev/null 2>&1 || {
    echo "Error: AWS credentials not configured. Run 'aws configure' first."
    echo ""
    echo "Required setup:"
    echo "  1. Install AWS CLI: https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html"
    echo "  2. Configure credentials: aws configure"
    echo "     - AWS Access Key ID: your-access-key"
    echo "     - AWS Secret Access Key: your-secret-key"
    echo "     - Default region: us-east-2"
    echo "     - Output format: json"
    echo ""
    echo "Or set environment variables:"
    echo "  export AWS_ACCESS_KEY_ID=your-access-key"
    echo "  export AWS_SECRET_ACCESS_KEY=your-secret-key"
    echo "  export AWS_DEFAULT_REGION=us-east-2"
    exit 1
}

ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
echo "Authenticated as account: $ACCOUNT_ID"
echo ""

# Create S3 bucket for state
echo "Creating S3 bucket: $BUCKET_NAME..."
if aws s3api head-bucket --bucket "$BUCKET_NAME" 2>/dev/null; then
    echo "  Bucket already exists"
else
    aws s3api create-bucket \
        --bucket "$BUCKET_NAME" \
        --region "$REGION" \
        --create-bucket-configuration LocationConstraint="$REGION"
    echo "  Created"
fi

# Enable versioning
echo "Enabling versioning on S3 bucket..."
aws s3api put-bucket-versioning \
    --bucket "$BUCKET_NAME" \
    --versioning-configuration Status=Enabled

# Enable encryption
echo "Enabling server-side encryption..."
aws s3api put-bucket-encryption \
    --bucket "$BUCKET_NAME" \
    --server-side-encryption-configuration '{
        "Rules": [{
            "ApplyServerSideEncryptionByDefault": {
                "SSEAlgorithm": "AES256"
            }
        }]
    }'

# Block public access
echo "Blocking public access..."
aws s3api put-public-access-block \
    --bucket "$BUCKET_NAME" \
    --public-access-block-configuration \
        BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true

echo ""
echo "S3 bucket configured: s3://$BUCKET_NAME"
echo ""

echo "=== Bootstrap Complete ==="
echo ""
echo "You can now run:"
echo "  cd infrastructure/environments/production"
echo "  terraform init"
echo "  terraform plan"
echo ""
