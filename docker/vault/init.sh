#!/bin/sh
# Initialize Vault for ZiraLoop development/testing
# This script runs after Vault dev server starts

set -e

export VAULT_ADDR=http://localhost:8200
export VAULT_TOKEN=ziraloop-dev-token

echo "Waiting for Vault to be ready..."
until vault status > /dev/null 2>&1; do
    sleep 1
done
echo "Vault is ready!"

# Enable the Transit engine if not already enabled
echo "Enabling Transit engine..."
vault secrets enable -path=transit transit || echo "Transit engine already enabled"

# Create the encryption key for envelope encryption
echo "Creating encryption key 'ziraloop-key'..."
vault write -f transit/keys/ziraloop-key || echo "Key already exists"

echo "Vault initialization complete!"
echo ""
echo "Vault Configuration for ZiraLoop:"
echo "  VAULT_ADDRESS: http://localhost:8200 (or http://vault:8200 from inside Docker)"
echo "  VAULT_TOKEN: ziraloop-dev-token"
echo "  KMS_KEY: ziraloop-key"
echo "  KMS_TYPE: vault"
