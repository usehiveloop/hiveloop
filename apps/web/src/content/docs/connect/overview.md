---
title: Connect Overview
description: LLMVault Connect is a pre-built UI widget that enables your users to securely connect their LLM providers and third-party integrations.
---

# Connect

LLMVault Connect is a pre-built, embeddable UI widget that enables your users to securely connect their LLM providers and third-party integrations to your application. It provides a seamless, white-labeled authentication experience without requiring you to build complex OAuth flows or API key management interfaces.

## What is Connect?

Connect is a React-based widget served from `https://connect.llmvault.dev` (production) or `https://connect.dev.llmvault.dev` (development). It runs in an iframe within your application and handles:

- **LLM Provider Connections**: Users can connect API keys from 20+ LLM providers including OpenAI, Anthropic, Google Gemini, Mistral, Groq, and more.
- **Third-party Integrations**: OAuth-based connections to services like Slack, GitHub, Notion, and other integrations powered by Nango.
- **Resource Selection**: Granular permission controls allowing users to select specific resources (channels, repositories, documents) that your application can access.
- **Connection Management**: View, verify, and revoke existing connections through an intuitive interface.

## Key Features

### For End Users
- **One-click provider selection** from popular LLM providers
- **Secure API key input** with encryption at rest
- **OAuth flows** for third-party integrations without leaving your app
- **Resource-level permissions** to control data access
- **Connection management** to view and revoke access

### For Developers
- **Zero UI development** — drop-in widget with minimal configuration
- **Session-based security** with short-lived tokens
- **Event-driven API** for handling connection success, errors, and closures
- **Customizable theming** to match your brand
- **TypeScript support** with full type definitions

## How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│                        Your Application                          │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              LLMVault Connect Widget (iframe)            │    │
│  │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐  │    │
│  │  │   Provider  │ -> │  API Key    │ -> │  Success    │  │    │
│  │  │  Selection  │    │   Input     │    │   Screen    │  │    │
│  │  └─────────────┘    └─────────────┘    └─────────────┘  │    │
│  │                                                         │    │
│  │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐  │    │
│  │  │ Integration │ -> │   OAuth     │ -> │  Resource   │  │    │
│  │  │  Selection  │    │   Auth      │    │  Selection  │  │    │
│  │  └─────────────┘    └─────────────┘    └─────────────┘  │    │
│  └─────────────────────────────────────────────────────────┘    │
│                              │                                   │
│                    postMessage Events                           │
│                              │                                   │
│                    ┌─────────▼──────────┐                       │
│                    │   onSuccess()      │                       │
│                    │   onError()        │                       │
│                    │   onClose()        │                       │
│                    └────────────────────┘                       │
└─────────────────────────────────────────────────────────────────┘
```

### Flow Overview

1. **Create a Session**: Your backend creates a Connect session via `POST /v1/connect/sessions` with user identity and permissions
2. **Open the Widget**: Use the Frontend SDK to open Connect with the session token
3. **User Authenticates**: User selects a provider, enters their API key or completes OAuth
4. **Key Verification**: API keys are validated with the provider before storage
5. **Event Handling**: Widget sends events (`success`, `error`, `close`) via postMessage
6. **Store Reference**: Your backend receives the connection ID for future API calls

## Security

### API Key Security
- **Encryption at rest**: All API keys are encrypted using AES-256-GCM with unique data encryption keys (DEK)
- **Envelope encryption**: DEKs are wrapped using AWS KMS or your configured key management service
- **Memory safety**: Keys are zeroed from memory immediately after use
- **Verification**: Keys are validated with the provider before storage (optional)

### Session Security
- **Short-lived tokens**: Sessions default to 15 minutes TTL (max 30 minutes)
- **Single-use tokens**: Session tokens are validated on every widget API call
- **Origin validation**: Optional allowed origins restrict where sessions can be used
- **Permission scoping**: Sessions can be restricted to specific operations (`create`, `list`, `delete`, `verify`)

### OAuth Security
- **Nango-powered**: OAuth flows are handled by Nango, a secure integration platform
- **No token exposure**: OAuth tokens are stored by Nango, never exposed to your frontend
- **Connect sessions**: Short-lived Nango connect sessions scope access to specific integrations

## Supported Providers

### LLM Providers (API Key)
Connect supports 20+ LLM providers with automatic base URL resolution and auth scheme detection:

| Provider | Auth Scheme | Base URL |
|----------|-------------|----------|
| OpenAI | Bearer | `https://api.openai.com` |
| Anthropic | X-API-Key | `https://api.anthropic.com` |
| Google Gemini | Query Param | `https://generativelanguage.googleapis.com` |
| Mistral | Bearer | `https://api.mistral.ai` |
| Groq | Bearer | `https://api.groq.com/openai` |
| Cohere | Bearer | `https://api.cohere.com` |
| Perplexity | Bearer | `https://api.perplexity.ai` |
| DeepSeek | Bearer | `https://api.deepseek.com` |
| Azure | API-Key | `https://models.inference.ai.azure.com` |
| AWS Bedrock | Bearer | `https://bedrock-runtime.us-east-1.amazonaws.com` |

See the [Providers](./providers) documentation for the complete list.

### Integration Providers (OAuth)
Any provider available in the Nango catalog can be configured:

- Slack, Discord, Microsoft Teams
- GitHub, GitLab, Bitbucket
- Notion, Confluence, Google Workspace
- Salesforce, HubSpot, Zendesk
- And 100+ more

See the [Integrations](./integrations) documentation for configuration details.

## Deployment

Connect is hosted on AWS with the following infrastructure:

```
User → connect.llmvault.dev
         ↓
    CloudFront (CDN + SSL)
         ↓
      S3 Bucket (Static Assets)
```

- **Production**: `https://connect.llmvault.dev`
- **Development**: `https://connect.dev.llmvault.dev`

## Next Steps

- [Embedding Connect](./embedding) — Learn how to embed the widget in your application
- [Session Management](./sessions) — Create and manage Connect sessions
- [Frontend SDK Reference](./frontend-sdk) — Complete SDK documentation
- [Theming](./theming) — Customize the widget appearance
