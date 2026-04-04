#!/bin/bash

# Atomic zero-downtime deployment for Connect app
# Uses versioned S3 folders + CloudFront origin path switching

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step() { echo -e "${CYAN}[STEP]${NC} $1"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="${SCRIPT_DIR}/.."

ENV="${1:-}"
if [[ -z "$ENV" ]]; then
    log_error "Environment not specified"
    echo "Usage: $0 <environment>"
    exit 1
fi

case "$ENV" in
    dev|development)
        ENV="dev"
        STACK_NAME="ZiraLoop-Dev-Connect"
        ;;
    prod|production)
        ENV="prod"
        STACK_NAME="ZiraLoop-Prod-Connect"
        ;;
    *)
        log_error "Unknown environment: $ENV"
        exit 1
        ;;
esac

# Generate version hash from git commit
VERSION=$(git -C "$APP_DIR" rev-parse --short HEAD 2>/dev/null || date +%s)
log_info "Deploying version: $VERSION"

# Get infrastructure details
log_step "Fetching infrastructure details..."
get_output() {
    aws cloudformation describe-stacks \
        --stack-name "$STACK_NAME" \
        --region us-east-2 \
        --query "Stacks[0].Outputs[?OutputKey=='$1'].OutputValue" \
        --output text
}

BUCKET_NAME=$(get_output "S3BucketName")
DISTRIBUTION_ID=$(get_output "DistributionId")

log_info "Bucket: $BUCKET_NAME"
log_info "Distribution: $DISTRIBUTION_ID"

# Build
log_step "Building app..."
cd "$APP_DIR"
if [[ "$ENV" == "prod" ]]; then
    npm run build:prod
else
    npm run build
fi

# Upload to versioned folder
log_step "Uploading to versioned folder: v$VERSION"
aws s3 sync dist/ "s3://${BUCKET_NAME}/v${VERSION}/" \
    --delete \
    --cache-control "max-age=31536000,immutable"

log_success "Version uploaded: s3://${BUCKET_NAME}/v${VERSION}/"

# Atomically update "current" symlink by copying index.html
log_step "Updating current version..."
aws s3 cp "s3://${BUCKET_NAME}/v${VERSION}/index.html" \
    "s3://${BUCKET_NAME}/index.html" \
    --cache-control "no-cache,no-store,must-revalidate"

# Invalidate CloudFront
log_step "Invalidating CloudFront cache..."
aws cloudfront create-invalidation \
    --distribution-id "$DISTRIBUTION_ID" \
    --paths "/index.html" \
    --query 'Invalidation.Id' --output text

log_success "Atomic deployment complete!"
log_info "Version: v${VERSION}"
log_info "URL: https://connect.${ENV}.ziraloop.com"

# NOTE: Old versions are kept for quick rollback
# To rollback: aws s3 cp s3://bucket/v{old-version}/index.html s3://bucket/index.html
# To list versions: aws s3 ls s3://${BUCKET_NAME}/ | grep "^PRE v"
