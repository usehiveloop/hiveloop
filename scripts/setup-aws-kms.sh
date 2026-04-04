#!/bin/bash
#
# setup-aws-kms.sh — Create KMS keys and IAM entities for ZiraLoop
#
# Usage:
#   ./setup-aws-kms.sh dev
#   ./setup-aws-kms.sh prod
#   ./setup-aws-kms.sh all
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_NAME="ziraloop"
ENVIRONMENT="${1:-}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# =============================================================================
# Helper Functions
# =============================================================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

check_aws_cli() {
    if ! command -v aws &> /dev/null; then
        log_error "AWS CLI is not installed. Please install it first:"
        echo "  https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html"
        exit 1
    fi

    if ! aws sts get-caller-identity &> /dev/null; then
        log_error "AWS CLI is not configured. Please run 'aws configure' first."
        exit 1
    fi

    local account_id
    account_id=$(aws sts get-caller-identity --query Account --output text)
    log_info "Using AWS Account: $account_id"
}

get_aws_region() {
    # Get region from AWS config, or use default
    aws configure get region || echo "us-east-1"
}

# =============================================================================
# KMS Key Creation
# =============================================================================

create_kms_key() {
    local env=$1
    local key_alias="alias/${PROJECT_NAME}-${env}-master"
    local key_description="ZiraLoop master key for credential encryption (${env} environment)"

    log_info "Creating KMS key for ${env} environment..."

    # Check if key already exists
    if aws kms describe-key --key-id "$key_alias" &> /dev/null; then
        log_warn "KMS key ${key_alias} already exists. Skipping creation."
        local existing_key_id
        existing_key_id=$(aws kms describe-key --key-id "$key_alias" --query 'KeyMetadata.KeyId' --output text)
        echo "$existing_key_id"
        return 0
    fi

    # Create the key
    local key_id
    key_id=$(aws kms create-key \
        --description "$key_description" \
        --key-usage ENCRYPT_DECRYPT \
        --origin AWS_KMS \
        --tags \
            TagKey=Environment,TagValue="$env" \
            TagKey=Application,TagValue="$PROJECT_NAME" \
            TagKey=ManagedBy,TagValue="script" \
        --query 'KeyMetadata.KeyId' \
        --output text)

    log_success "Created KMS key: $key_id"

    # Create alias
    aws kms create-alias \
        --alias-name "$key_alias" \
        --target-key-id "$key_id"

    log_success "Created alias: $key_alias"

    echo "$key_id"
}

enable_key_rotation() {
    local key_id=$1
    local env=$2

    log_info "Enabling automatic key rotation for ${env}..."

    aws kms enable-key-rotation --key-id "$key_id"

    log_success "Automatic key rotation enabled"
}

# =============================================================================
# IAM User Setup (for external application access)
# =============================================================================

create_iam_user() {
    local env=$1
    local username="${PROJECT_NAME}-${env}-app"

    log_info "Creating IAM user for ${env}..."

    # Check if user exists
    if aws iam get-user --user-name "$username" &> /dev/null; then
        log_warn "IAM user ${username} already exists."
    else
        aws iam create-user \
            --user-name "$username" \
            --tags Key=Environment,Value="$env" Key=Application,Value="$PROJECT_NAME" > /dev/null
        log_success "Created IAM user: $username"
    fi

    echo "$username"
}

create_kms_policy() {
    local env=$1
    local key_arn=$2
    local policy_name="${PROJECT_NAME}-${env}-kms-access"

    log_info "Creating IAM policy for ${env} KMS access..."

    # Policy document
    local policy_document
    policy_document=$(cat <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "kms:Encrypt",
                "kms:Decrypt",
                "kms:ReEncryptFrom",
                "kms:ReEncryptTo",
                "kms:GenerateDataKey",
                "kms:GenerateDataKeyWithoutPlaintext",
                "kms:DescribeKey"
            ],
            "Resource": "${key_arn}"
        }
    ]
}
EOF
)

    # Check if policy exists
    local policy_arn
    policy_arn=$(aws iam list-policies --scope Local --query "Policies[?PolicyName=='${policy_name}'].Arn" --output text)

    if [[ -n "$policy_arn" && "$policy_arn" != "None" ]]; then
        log_warn "Policy ${policy_name} already exists. Updating..."
        
        # Get current version
        local current_version
        current_version=$(aws iam get-policy --policy-arn "$policy_arn" --query 'Policy.DefaultVersionId' --output text)
        
        # Create new version
        aws iam create-policy-version \
            --policy-arn "$policy_arn" \
            --policy-document "$policy_document" \
            --set-as-default || true
        
        # Delete old versions (keep only the last 2)
        local versions
        versions=$(aws iam list-policy-versions --policy-arn "$policy_arn" --query 'Versions[?!IsDefaultVersion].VersionId' --output text)
        for version in $versions; do
            aws iam delete-policy-version --policy-arn "$policy_arn" --version-id "$version" 2>/dev/null || true
        done
    else
        policy_arn=$(aws iam create-policy \
            --policy-name "$policy_name" \
            --description "KMS access for ZiraLoop ${env} environment" \
            --policy-document "$policy_document" \
            --tags Key=Environment,Value="$env" Key=Application,Value="$PROJECT_NAME" \
            --query 'Policy.Arn' \
            --output text)
        log_success "Created IAM policy: $policy_name"
    fi

    echo "$policy_arn"
}

attach_policy_to_user() {
    local username=$1
    local policy_arn=$2
    local env=$3

    log_info "Attaching KMS policy to ${username}..."

    aws iam attach-user-policy \
        --user-name "$username" \
        --policy-arn "$policy_arn" > /dev/null 2>&1 || log_warn "Policy already attached"

    log_success "Policy attached"
}

create_access_keys() {
    local username=$1
    local env=$2

    log_info "Creating access keys for ${username}..."
    log_warn "Save these credentials securely! They will not be shown again."

    local credentials
    credentials=$(aws iam create-access-key --user-name "$username")

    local access_key_id
    access_key_id=$(echo "$credentials" | jq -r '.AccessKey.AccessKeyId')
    
    local secret_access_key
    secret_access_key=$(echo "$credentials" | jq -r '.AccessKey.SecretAccessKey')

    # Save to file
    local output_file="${SCRIPT_DIR}/aws-credentials-${env}.env"
    cat > "$output_file" <<EOF
# ZiraLoop AWS Credentials - ${env} environment
# Generated: $(date)
# User: ${username}

# AWS Credentials
AWS_ACCESS_KEY_ID=${access_key_id}
AWS_SECRET_ACCESS_KEY=${secret_access_key}
AWS_REGION=$(get_aws_region)

# ZiraLoop Configuration
KMS_TYPE=awskms
KMS_KEY=alias/${PROJECT_NAME}-${env}-master
EOF

    chmod 600 "$output_file"
    log_success "Credentials saved to: $output_file"
    log_warn "IMPORTANT: This file contains secrets. Keep it secure!"
}

# =============================================================================
# Output Summary
# =============================================================================

print_summary() {
    local env=$1
    local key_id=$2
    local key_arn=$3
    local username=$4
    local region
    region=$(get_aws_region)

    echo ""
    echo "=========================================="
    echo "  ZiraLoop AWS Setup - ${env}"
    echo "=========================================="
    echo ""
    echo "KMS Key ID:     ${key_id}"
    echo "KMS Key ARN:    ${key_arn}"
    echo "KMS Alias:      alias/${PROJECT_NAME}-${env}-master"
    echo "AWS Region:     ${region}"
    echo "IAM User:       ${username}"
    echo ""
    echo "Environment Variables:"
    echo "  KMS_TYPE=awskms"
    echo "  KMS_KEY=alias/${PROJECT_NAME}-${env}-master"
    echo "  AWS_REGION=${region}"
    echo ""
    echo "Credentials file: aws-credentials-${env}.env"
    echo ""
    echo "To use these credentials:"
    echo "  source ${SCRIPT_DIR}/aws-credentials-${env}.env"
    echo ""
    echo "=========================================="
}

# =============================================================================
# Environment Setup
# =============================================================================

setup_environment() {
    local env=$1

    echo ""
    echo "=========================================="
    echo "  Setting up ${env} environment"
    echo "=========================================="

    # Create KMS key
    local key_id
    key_id=$(create_kms_key "$env")

    local key_arn
    key_arn=$(aws kms describe-key --key-id "$key_id" --query 'KeyMetadata.Arn' --output text)

    # Enable key rotation for production
    if [[ "$env" == "prod" ]]; then
        enable_key_rotation "$key_id" "$env"
    fi

    # Create IAM user
    local username
    username=$(create_iam_user "$env")

    # Create and attach policy
    local policy_arn
    policy_arn=$(create_kms_policy "$env" "$key_arn")
    attach_policy_to_user "$username" "$policy_arn" "$env"

    # Create access keys
    read -p "Create new access keys for ${env}? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        create_access_keys "$username" "$env"
    fi

    # Print summary
    print_summary "$env" "$key_id" "$key_arn" "$username"
}

# =============================================================================
# Main
# =============================================================================

main() {
    echo "=========================================="
    echo "  ZiraLoop AWS Infrastructure Setup"
    echo "=========================================="

    # Validate arguments
    if [[ -z "$ENVIRONMENT" ]]; then
        echo "Usage: $0 <dev|prod|all>"
        echo ""
        echo "Examples:"
        echo "  $0 dev     # Setup development environment"
        echo "  $0 prod    # Setup production environment"
        echo "  $0 all     # Setup both environments"
        exit 1
    fi

    # Check dependencies
    check_aws_cli

    if ! command -v jq &> /dev/null; then
        log_error "jq is required but not installed. Please install it:"
        echo "  macOS: brew install jq"
        echo "  Ubuntu/Debian: sudo apt-get install jq"
        exit 1
    fi

    # Confirm before production setup
    if [[ "$ENVIRONMENT" == "prod" || "$ENVIRONMENT" == "all" ]]; then
        echo ""
        log_warn "You are about to create PRODUCTION infrastructure."
        log_warn "This will incur AWS costs and create resources that should be carefully managed."
        read -p "Are you sure you want to continue? (yes/no): " confirm
        if [[ "$confirm" != "yes" ]]; then
            log_info "Aborted."
            exit 0
        fi
    fi

    # Run setup
    case "$ENVIRONMENT" in
        dev)
            setup_environment "dev"
            ;;
        prod)
            setup_environment "prod"
            ;;
        all)
            setup_environment "dev"
            setup_environment "prod"
            ;;
        *)
            log_error "Unknown environment: $ENVIRONMENT"
            echo "Valid options: dev, prod, all"
            exit 1
            ;;
    esac

    echo ""
    log_success "Setup complete!"
}

main "$@"
