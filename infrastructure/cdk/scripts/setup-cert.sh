#!/bin/bash

# Setup ACM certificate for CloudFront custom domain
# 
# Usage:
#   ./scripts/setup-cert.sh <environment>
#
# Environment:
#   - dev:    Sets up cert for connect.dev.llmvault.dev
#   - prod:   Sets up cert for connect.llmvault.dev

set -euo pipefail

ENV="${1:-}"

if [[ -z "$ENV" ]]; then
    echo "Error: Environment not specified"
    echo ""
    echo "Usage: $0 <environment>"
    echo ""
    echo "Environments:"
    echo "  dev   - Setup cert for connect.dev.llmvault.dev"
    echo "  prod  - Setup cert for connect.llmvault.dev"
    exit 1
fi

case "$ENV" in
    dev|development)
        DOMAIN="connect.dev.llmvault.dev"
        ;;
    prod|production)
        DOMAIN="connect.llmvault.dev"
        ;;
    *)
        echo "Error: Unknown environment: $ENV"
        echo "Valid environments: dev, prod"
        exit 1
        ;;
esac

echo "========================================"
echo "  Setup ACM Certificate"
echo "  Domain: $DOMAIN"
echo "  Region: us-east-1 (required by CloudFront)"
echo "========================================"
echo ""

# Check if cert already exists
echo "Checking for existing certificate..."
EXISTING_CERT=$(aws acm list-certificates --region us-east-1 \
    --query "CertificateSummaryList[?DomainName=='$DOMAIN'].CertificateArn" \
    --output text)

if [[ -n "$EXISTING_CERT" && "$EXISTING_CERT" != "None" ]]; then
    echo "Certificate already exists:"
    echo "  ARN: $EXISTING_CERT"
    
    # Get status
    STATUS=$(aws acm describe-certificate --region us-east-1 \
        --certificate-arn "$EXISTING_CERT" \
        --query 'Certificate.Status' --output text)
    echo "  Status: $STATUS"
    
    if [[ "$STATUS" == "ISSUED" ]]; then
        echo ""
        echo "✅ Certificate is ready to use!"
        echo ""
        echo "Add this ARN to your CDK config or update CloudFront:"
        echo "  $EXISTING_CERT"
        exit 0
    fi
    
    echo ""
    echo "⏳ Certificate exists but not yet validated."
    echo "Check DNS validation records in Cloudflare."
    exit 0
fi

# Request new certificate
echo "Requesting new certificate..."
CERT_ARN=$(aws acm request-certificate --region us-east-1 \
    --domain-name "$DOMAIN" \
    --validation-method DNS \
    --query 'CertificateArn' --output text)

echo "Certificate requested: $CERT_ARN"
echo ""

# Get validation details
echo "Waiting for validation info (may take a few seconds)..."
sleep 5

VALIDATION_INFO=$(aws acm describe-certificate --region us-east-1 \
    --certificate-arn "$CERT_ARN" \
    --query 'Certificate.DomainValidationOptions[0]' \
    --output json)

echo ""
echo "========================================"
echo "  DNS Validation Required"
echo "========================================"
echo ""
echo "Add this CNAME record to Cloudflare:"
echo ""

# Parse validation details
VALIDATION_DOMAIN=$(echo "$VALIDATION_INFO" | jq -r '.ResourceRecord.Name')
VALIDATION_VALUE=$(echo "$VALIDATION_INFO" | jq -r '.ResourceRecord.Value')

echo "  Type:  CNAME"
echo "  Name:  $VALIDATION_DOMAIN"
echo "  Value: $VALIDATION_VALUE"
echo ""

echo "After adding the DNS record, validation may take 5-30 minutes."
echo ""
echo "Certificate ARN (save this!):"
echo "  $CERT_ARN"
echo ""

# Optional: Wait for validation
echo "Do you want to wait for validation? (y/N)"
read -r WAIT_FOR_VALIDATION

if [[ "$WAIT_FOR_VALIDATION" =~ ^[Yy]$ ]]; then
    echo "Waiting for validation (this may take several minutes)..."
    aws acm wait certificate-validated --region us-east-1 --certificate-arn "$CERT_ARN"
    echo "✅ Certificate validated!"
else
    echo ""
    echo "To check validation status later, run:"
    echo "  aws acm describe-certificate --region us-east-1 --certificate-arn $CERT_ARN --query 'Certificate.Status'"
fi
