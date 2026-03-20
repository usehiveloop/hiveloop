---
title: Installation
description: Install LLMVault SDKs for TypeScript, Python, Go, and Frontend frameworks.
---

# Installation

LLMVault provides official SDKs for popular programming languages. Choose the SDK that fits your stack.

## TypeScript / JavaScript

The server-side SDK for Node.js applications.

### npm

```bash
npm install @llmvault/sdk
```

### yarn

```bash
yarn add @llmvault/sdk
```

### pnpm

```bash
pnpm add @llmvault/sdk
```

### Usage

```typescript
import { LLMVault } from '@llmvault/sdk';

const vault = new LLMVault({
  apiKey: process.env.LLMVAULT_API_KEY,
  // Optional: override base URL for self-hosted instances
  // baseUrl: 'https://api.llmvault.dev'
});

// List credentials
const { data, error } = await vault.credentials.list();

// Create a credential
const { data: credential } = await vault.credentials.create({
  label: 'production_openai',
  provider_id: 'openai',
  base_url: 'https://api.openai.com',
  auth_scheme: 'bearer',
  api_key: 'sk-...'
});

// Mint a token
const { data: token } = await vault.tokens.create({
  credential_id: credential.id,
  ttl: '1h'
});
```

### Requirements

- Node.js 18 or higher
- TypeScript 5.0+ (for TypeScript projects)

## Python

The Python SDK provides async/await support and sync clients.

### pip

```bash
pip install llmvault
```

### poetry

```bash
poetry add llmvault
```

### Usage

```python
from llmvault import LLMVault

vault = LLMVault(api_key="llmv_sk_...")

# List credentials
credentials = vault.credentials.list()

# Create a credential
credential = vault.credentials.create(
    label="production_openai",
    provider_id="openai",
    base_url="https://api.openai.com",
    auth_scheme="bearer",
    api_key="sk-..."
)

# Mint a token
token = vault.tokens.create(
    credential_id=credential.id,
    ttl="1h"
)
```

### Async Usage

```python
import asyncio
from llmvault import AsyncLLMVault

async def main():
    vault = AsyncLLMVault(api_key="llmv_sk_...")
    
    credentials = await vault.credentials.list()
    print(credentials)

asyncio.run(main())
```

### Requirements

- Python 3.9 or higher

## Go

The Go SDK provides a typed client for LLMVault API.

### go get

```bash
go get github.com/llmvault/llmvault-go
```

### Usage

```go
package main

import (
    "context"
    "log"
    
    "github.com/llmvault/llmvault-go"
)

func main() {
    client := llmvault.NewClient("llmv_sk_...")
    
    ctx := context.Background()
    
    // List credentials
    credentials, err := client.Credentials.List(ctx, nil)
    if err != nil {
        log.Fatal(err)
    }
    
    // Create a credential
    credential, err := client.Credentials.Create(ctx, llmvault.CreateCredentialRequest{
        Label:      "production_openai",
        ProviderID: "openai",
        BaseURL:    "https://api.openai.com",
        AuthScheme: "bearer",
        APIKey:     "sk-...",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Mint a token
    token, err := client.Tokens.Create(ctx, llmvault.MintTokenRequest{
        CredentialID: credential.ID,
        TTL:          "1h",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Token: %s", token.Token)
}
```

### Requirements

- Go 1.21 or higher

## Frontend SDK

The Frontend SDK is a lightweight client for embedding the LLMVault Connect widget in browser applications.

### npm / yarn / pnpm

```bash
npm install @llmvault/frontend
```

### Usage

```typescript
import { LLMVaultConnect } from '@llmvault/frontend';

const connect = new LLMVaultConnect({
  baseURL: 'https://connect.llmvault.dev',
  theme: 'system' // 'light' | 'dark' | 'system'
});

// Open the connect widget
connect.open({
  sessionToken: 'csess_...', // From your backend
  screen: 'provider-selection', // Optional
  onSuccess: (payload) => {
    console.log('Connected:', payload.providerId, payload.connectionId);
  },
  onError: (error) => {
    console.error('Connection failed:', error.message);
  },
  onClose: () => {
    console.log('Widget closed');
  }
});

// Close the widget programmatically
connect.close();

// Check if widget is open
console.log(connect.isOpen);
```

### React Example

```tsx
import { useState } from 'react';
import { LLMVaultConnect } from '@llmvault/frontend';

const connect = new LLMVaultConnect({ theme: 'system' });

function ConnectButton({ sessionToken }: { sessionToken: string }) {
  const [isOpen, setIsOpen] = useState(false);

  const handleConnect = () => {
    connect.open({
      sessionToken,
      onSuccess: (payload) => {
        console.log('Connected!', payload);
        setIsOpen(false);
      },
      onClose: () => setIsOpen(false)
    });
    setIsOpen(true);
  };

  return (
    <button onClick={handleConnect} disabled={isOpen}>
      {isOpen ? 'Connecting...' : 'Connect LLM Provider'}
    </button>
  );
}
```

### Vue Example

```vue
<template>
  <button @click="openConnect" :disabled="isOpen">
    {{ isOpen ? 'Connecting...' : 'Connect LLM Provider' }}
  </button>
</template>

<script setup>
import { ref } from 'vue';
import { LLMVaultConnect } from '@llmvault/frontend';

const connect = new LLMVaultConnect({ theme: 'system' });
const isOpen = ref(false);

const openConnect = () => {
  connect.open({
    sessionToken: props.sessionToken,
    onSuccess: (payload) => {
      console.log('Connected!', payload);
      isOpen.value = false;
    },
    onClose: () => {
      isOpen.value = false;
    }
  });
  isOpen.value = true;
};
</script>
```

### Browser Support

- Chrome 90+
- Firefox 88+
- Safari 14+
- Edge 90+

## Environment Variables

Configure your SDK with environment variables:

```bash
# Required: Your LLMVault API key
LLMVAULT_API_KEY=llmv_sk_...

# Optional: Override base URL for self-hosted
LLMVAULT_BASE_URL=https://api.llmvault.dev
```

## SDK Reference

### TypeScript SDK Resources

| Resource | Methods |
|----------|---------|
| `vault.apiKeys` | `create()`, `list()`, `delete(id)` |
| `vault.credentials` | `create()`, `list()`, `get(id)`, `delete(id)` |
| `vault.tokens` | `create()`, `list()`, `delete(jti)` |
| `vault.identities` | `create()`, `list()`, `get(id)`, `update(id)`, `delete(id)` |
| `vault.connect` | `sessions.create()`, `settings.get()`, `settings.update()` |
| `vault.integrations` | `create()`, `list()`, `get(id)`, `update(id)`, `delete(id)` |
| `vault.connections` | `availableScopes()`, `create(integrationId, body)`, `list(integrationId)`, `get(id)`, `retrieveToken(id)`, `delete(id)` |
| `vault.usage` | `get()` |
| `vault.audit` | `list()` |
| `vault.org` | `getCurrent()` |
| `vault.providers` | `list()`, `get(id)`, `listModels(id)` |

### TypeScript Types

```typescript
import type {
  LLMVaultConfig,
  CreateCredentialRequest,
  CreateAPIKeyRequest,
  MintTokenRequest,
  CreateConnectSessionRequest,
  // ... and more
} from '@llmvault/sdk';
```

## Next Steps

- Read the [Authentication guide](./authentication) to understand API keys, proxy tokens, and Connect sessions
- Follow the [Quickstart](./quickstart) for a complete walkthrough
- Explore the [API reference](/docs/api) for all available endpoints
