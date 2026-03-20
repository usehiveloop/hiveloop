---
title: Embedding Connect
description: Learn how to embed the LLMVault Connect widget in your application using the Frontend SDK, session tokens, and event handling.
---

# Embedding Connect

This guide covers embedding the Connect widget in your application, from creating sessions to handling user interactions.

## Prerequisites

Before embedding Connect, you need:

1. **LLMVault Account** with API credentials
2. **Frontend SDK** installed (`@llmvault/frontend`)
3. **Backend endpoint** to create Connect sessions

## Installation

```bash
npm install @llmvault/frontend
```

## Quick Start

### 1. Create a Session (Backend)

First, create a Connect session on your backend:

```typescript
// Server-side: Create a Connect session
const response = await fetch('https://api.llmvault.dev/v1/connect/sessions', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${LLMVAULT_API_KEY}`,
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    external_id: 'user-123',           // Your user's ID
    permissions: ['create', 'list'],   // Allowed operations
    ttl: '15m',                        // Session TTL (max 30m)
  }),
});

const { session_token } = await response.json();
// Pass session_token to your frontend
```

### 2. Embed the Widget (Frontend)

```typescript
import { LLMVaultConnect } from '@llmvault/frontend';

// Initialize Connect
const connect = new LLMVaultConnect({
  theme: 'system',  // 'light' | 'dark' | 'system'
});

// Open the widget
connect.open({
  sessionToken: 'sess_xxx',  // From your backend
  onSuccess: (payload) => {
    console.log('Connected:', payload.providerId, payload.connectionId);
  },
  onError: (error) => {
    console.error('Error:', error.code, error.message);
  },
  onClose: () => {
    console.log('Widget closed');
  },
});
```

## Session Tokens

Session tokens are short-lived, single-use tokens that authorize the widget to act on behalf of your user.

### Creating Sessions

```http
POST /v1/connect/sessions
Content-Type: application/json
Authorization: Bearer {api_key}

{
  "external_id": "user-123",
  "identity_id": "550e8400-e29b-41d4-a716-446655440000",
  "permissions": ["create", "list", "delete", "verify"],
  "allowed_integrations": ["slack-prod", "github-team"],
  "allowed_origins": ["https://app.example.com"],
  "ttl": "15m",
  "metadata": {
    "plan": "enterprise",
    "region": "us-east"
  }
}
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `external_id` | string | Conditionally | Your user's unique identifier. Required if `identity_id` not provided. |
| `identity_id` | UUID | Conditionally | Existing LLMVault identity ID. Required if `external_id` not provided. |
| `permissions` | string[] | No | Allowed operations: `create`, `list`, `delete`, `verify`. Default: all. |
| `allowed_integrations` | string[] | No | Restrict to specific integration unique keys |
| `allowed_origins` | string[] | No | Valid origins for this session (must match org's allowed_origins) |
| `ttl` | duration | No | Session lifetime (max 30m). Default: 15m |
| `metadata` | object | No | Custom data stored with the session |

**Response:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "session_token": "connect_session_xxx",
  "external_id": "user-123",
  "allowed_integrations": ["slack-prod"],
  "allowed_origins": ["https://app.example.com"],
  "expires_at": "2026-03-20T09:00:00Z",
  "created_at": "2026-03-20T08:45:00Z"
}
```

### Session Validation

The widget validates sessions on load:

```
GET /v1/widget/session
Authorization: Bearer {session_token}
```

If the session is invalid or expired, the widget displays an error:

```
┌─────────────────────────────┐
│        Session invalid       │
│                              │
│ This session has expired or │
│ is invalid. Please close and│
│ try again.                   │
│                              │
│         [Close]             │
└─────────────────────────────┘
```

## Event Handling

Connect communicates with your application via `postMessage` events. All events include a `type` field.

### Event Types

| Event | Payload | Description |
|-------|---------|-------------|
| `success` | `{ providerId, connectionId }` | LLM provider connected successfully |
| `integration_success` | `{ integrationId, connectionId }` | OAuth integration connected |
| `resource_selection` | `{ integrationId, resources }` | User selected resources |
| `error` | `{ code, message, providerId? }` | An error occurred |
| `close` | — | Widget was closed |

### Handling Success Events

```typescript
connect.open({
  sessionToken: 'sess_xxx',
  
  // LLM Provider connection success
  onSuccess: (payload) => {
    console.log('Provider:', payload.providerId);
    console.log('Connection ID:', payload.connectionId);
    
    // Store connection reference in your database
    await saveConnection(userId, payload);
  },
  
  // Integration connection success
  onIntegrationSuccess: (payload) => {
    console.log('Integration:', payload.integrationId);
    console.log('Connection ID:', payload.connectionId);
  },
  
  // Resource selection completed
  onResourceSelection: (payload) => {
    console.log('Integration:', payload.integrationId);
    console.log('Resources:', payload.resources);
    // Example: { "channels": ["C123", "C456"], "users": ["U789"] }
  },
});
```

### Handling Error Events

```typescript
connect.open({
  sessionToken: 'sess_xxx',
  
  onError: (error) => {
    console.error('Error Code:', error.code);
    console.error('Message:', error.message);
    console.error('Provider:', error.providerId); // Optional
    
    // Handle specific error codes
    switch (error.code) {
      case 'session_invalid':
        // Session expired, create a new one
        break;
      case 'connection_failed':
        // API key verification failed
        break;
      case 'integration_failed':
        // OAuth flow failed
        break;
      case 'unknown_error':
        // Unexpected error
        break;
    }
  },
});
```

**Error Codes:**

| Code | Description |
|------|-------------|
| `session_invalid` | Session expired or invalid |
| `session_expired` | Session TTL reached |
| `connection_failed` | API key verification failed |
| `integration_failed` | OAuth authentication failed |
| `unknown_error` | Unexpected error |

### Global Event Handler

Use `onEvent` to handle all events in one place:

```typescript
connect.open({
  sessionToken: 'sess_xxx',
  
  onEvent: (event) => {
    switch (event.type) {
      case 'success':
        // Handle provider connection
        break;
      case 'integration_success':
        // Handle integration connection
        break;
      case 'resource_selection':
        // Handle resource selection
        break;
      case 'error':
        // Handle error
        break;
      case 'close':
        // Handle close
        break;
    }
  },
});
```

## Error Handling

### SDK Errors

The SDK throws `ConnectError` for configuration issues:

```typescript
import { ConnectError } from '@llmvault/frontend';

try {
  connect.open({ sessionToken: '' });
} catch (error) {
  if (error instanceof ConnectError) {
    console.log(error.type);    // 'session_token_missing'
    console.log(error.message); // 'A session token is required...'
  }
}
```

**SDK Error Types:**

| Type | Description |
|------|-------------|
| `already_open` | Widget is already open |
| `session_token_missing` | No session token provided |
| `iframe_blocked` | iframe was blocked by browser |

### Widget Errors

Widget-level errors (API failures, network issues) are reported via `onError`:

```typescript
connect.open({
  sessionToken: 'sess_xxx',
  
  onError: (error) => {
    // Log to your error tracking service
    Sentry.captureException(new Error(error.message), {
      tags: { code: error.code },
    });
    
    // Show user-friendly message
    toast.error(`Connection failed: ${error.message}`);
  },
});
```

### Network Errors

The widget handles network failures gracefully:

```
┌─────────────────────────────┐
│    Unable to load providers  │
│                              │
│ We couldn't reach the server│
│ to load available providers.│
│ Please check your connection│
│ and try again.              │
│                              │
│    [Retry]      [Cancel]    │
└─────────────────────────────┘
```

## Opening Specific Screens

Skip the selection screen and open directly to a specific flow:

```typescript
// Direct provider connection
connect.open({
  sessionToken: 'sess_xxx',
  screen: 'provider-connect',
  providerId: 'openai',
});

// Direct integration connection
connect.open({
  sessionToken: 'sess_xxx',
  screen: 'integration-connect',
  integrationId: 'slack-prod',
});

// Show connected providers list
connect.open({
  sessionToken: 'sess_xxx',
  screen: 'connected-list',
});
```

**Available Screens:**

| Screen | Description |
|--------|-------------|
| `provider-selection` | Default - choose from available LLM providers |
| `integration-selection` | Choose from configured integrations |
| `connected-list` | View and manage existing connections |
| `provider-connect` | Direct to specific provider (requires `providerId`) |
| `integration-connect` | Direct to specific integration (requires `integrationId`) |

## Complete Example

```typescript
import { LLMVaultConnect, ConnectError } from '@llmvault/frontend';

class ConnectManager {
  private connect: LLMVaultConnect;
  
  constructor() {
    this.connect = new LLMVaultConnect({
      theme: 'system',
    });
  }
  
  async openConnectionDialog(userId: string) {
    try {
      // 1. Create session from your backend
      const sessionToken = await this.createSession(userId);
      
      // 2. Open Connect widget
      this.connect.open({
        sessionToken,
        screen: 'provider-selection',
        
        onSuccess: async (payload) => {
          console.log(`Connected to ${payload.providerId}`);
          
          // Save to your database
          await fetch('/api/connections', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              userId,
              providerId: payload.providerId,
              connectionId: payload.connectionId,
            }),
          });
          
          // Refresh UI
          this.emit('connection:created', payload);
        },
        
        onIntegrationSuccess: async (payload) => {
          console.log(`Connected to ${payload.integrationId}`);
        },
        
        onResourceSelection: async (payload) => {
          console.log('Selected resources:', payload.resources);
        },
        
        onError: (error) => {
          console.error('Connect error:', error);
          this.emit('connection:error', error);
        },
        
        onClose: () => {
          console.log('Connect widget closed');
        },
      });
      
    } catch (error) {
      if (error instanceof ConnectError) {
        console.error('SDK Error:', error.type, error.message);
      } else {
        console.error('Unexpected error:', error);
      }
    }
  }
  
  private async createSession(userId: string): Promise<string> {
    const response = await fetch('/api/connect-session', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ userId }),
    });
    
    if (!response.ok) {
      throw new Error('Failed to create session');
    }
    
    const { session_token } = await response.json();
    return session_token;
  }
  
  close() {
    this.connect.close();
  }
  
  get isOpen(): boolean {
    return this.connect.isOpen;
  }
}
```

## Next Steps

- [Frontend SDK Reference](./frontend-sdk) — Complete SDK API documentation
- [Theming](./theming) — Customize widget appearance
- [Sessions](./sessions) — Advanced session management
- [Providers](./providers) — LLM provider configuration
