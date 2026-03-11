#!/bin/bash

# Update CloudFront distribution with custom domain and certificate
#
# Usage:
#   ./scripts/update-cloudfront.sh <environment> <certificate-arn>
#
# Example:
#   ./scripts/update-cloudfront.sh dev arn:aws:acm:us-east-1:123:certificate/abc

set -euo pipefail

ENV="${1:-}"
CERT_ARN="${2:-}"

if [[ -z "$ENV" || -z "$CERT_ARN" ]]; then
    echo "Error: Missing arguments"
    echo ""
    echo "Usage: $0 <environment> <certificate-arn>"
    echo ""
    echo "Example:"
    echo "  $0 dev arn:aws:acm:us-east-1:944765969440:certificate/abc123"
    exit 1
fi

case "$ENV" in
    dev|development)
        DOMAIN="connect.dev.llmvault.dev"
        DIST_ID="E1SRM0XYQG4Y06"  # Get from CDK output
        ;;
    prod|production)
        DOMAIN="connect.llmvault.dev"
        DIST_ID=""  # Update after prod deployment
        ;;
    *)
        echo "Error: Unknown environment: $ENV"
        echo "Valid environments: dev, prod"
        exit 1
        ;;
esac

# If DIST_ID is empty, try to get from CloudFormation
if [[ -z "$DIST_ID" ]]; then
    STACK_NAME="LlmVault-$(echo $ENV | sed 's/.*/\u&/')-Connect"
    DIST_ID=$(aws cloudformation describe-stacks \
        --stack-name "$STACK_NAME" \
        --region us-east-2 \
        --query 'Stacks[0].Outputs[?OutputKey==`DistributionId`].OutputValue' \
        --output text 2>/dev/null || echo "")
fi

if [[ -z "$DIST_ID" ]]; then
    echo "Error: Could not find Distribution ID"
    echo "Make sure the infrastructure stack is deployed"
    exit 1
fi

echo "========================================"
echo "  Update CloudFront Distribution"
echo "========================================"
echo ""
echo "Environment: $ENV"
echo "Domain:      $DOMAIN"
echo "Dist ID:     $DIST_ID"
echo "Cert ARN:    $CERT_ARN"
echo ""

# Get current config and ETag
echo "Fetching current distribution config..."
CONFIG=$(aws cloudfront get-distribution-config --id "$DIST_ID")
ETAG=$(echo "$CONFIG" | jq -r '.ETag')

echo "Current ETag: $ETAG"
echo ""

# Create updated config
UPDATED_CONFIG=$(echo "$CONFIG" | jq '
    .DistributionConfig 
    | .Aliases = {
        "Quantity": 1,
        "Items": ["'"$DOMAIN"'"]
    }
    | .ViewerCertificate = {
        "ACMCertificateArn": "'"$CERT_ARN"'",
        "SSLSupportMethod": "sni-only",
        "MinimumProtocolVersion": "TLSv1.2_2021",
        "Certificate": "'"$CERT_ARN"'",
        "CertificateSource": "acm"
    }
')

# Save to temp file
TEMP_FILE=$(mktemp)
echo "$UPDATED_CONFIG" > "$TEMP_FILE"

echo "Updating distribution..."
aws cloudfront update-distribution \
    --id "$DIST_ID" \
    --distribution-config "file://$TEMP_FILE" \
    --if-match "$ETAG"

rm "$TEMP_FILE"

echo ""
echo "✅ Update submitted!"
echo ""
echo "CloudFront is deploying (takes 5-10 minutes)..."
echo ""
echo "To wait for completion:"
echo "  aws cloudfront wait distribution-deployed --id $DIST_ID"
echo ""
echo "After deployment, add this DNS record to Cloudflare:"
echo ""
echo "  Type:  CNAME"
echo "  Name:  $([[ "$ENV" == "dev" ]] && echo "connect.dev" || echo "connect")"
echo "  Target: $(echo "$CONFIG" | jq -r '.DistributionConfig.DomainName')"
echo ""
