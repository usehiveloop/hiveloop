---
title: Frontend SDK
description: Frontend SDK for embedding the LLMVault Connect widget
---

The Frontend SDK provides a simple way to embed the LLMVault Connect widget in your web application, allowing end users to securely connect their LLM provider accounts and OAuth integrations.

## Installation

### NPM

```bash
npm install @llmvault/frontend
```

### CDN

```html
<script src="https://unpkg.com/@llmvault/frontend@latest/dist/index.global.js"></script>
```

## Quick Start

```typescript
import { LLMVaultConnect } from '@llmvault/frontend'

const connect = new LLMVaultConnect()

// Open the Connect widget
connect.open({
  sessionToken: 'csess_...', // From your backend
  onSuccess: (payload) => {
    console.log('Connected:', payload.providerId, payload.connectionId)
  },
  onClose: () => {
    console.log('Widget closed')
  }
})
```

## Configuration

### `LLMVaultConnectConfig`

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `baseURL` | `string` | No | Connect widget URL. Defaults to `https://connect.llmvault.dev` |
| `theme` | `ThemeOption` | No | Widget theme: `'light'`, `'dark'`, or `'system'`. Defaults to `'system'` |

```typescript
import { LLMVaultConnect } from '@llmvault/frontend'

const connect = new LLMVaultConnect({
  baseURL: 'https://connect.llmvault.dev',  // Optional
  theme: 'dark'                              // Optional
})
```

## API Reference

### Constructor

Creates a new LLMVaultConnect instance.

```typescript
const connect = new LLMVaultConnect(config?)
```

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `config` | `LLMVaultConnectConfig` | No | Configuration options |

**Example:**

```typescript
// Default configuration
const connect = new LLMVaultConnect()

// Custom configuration
const connect = new LLMVaultConnect({
  baseURL: 'https://custom.llmvault.dev',
  theme: 'light'
})
```

---

### `open(options)`

Opens the Connect widget as a full-screen iframe overlay.

```typescript
connect.open(options: ConnectOpenOptions): void
```

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `sessionToken` | `string` | **Yes** | Session token from your backend (`csess_...`) |
| `screen` | `ConnectScreen` | No | Initial screen to display |
| `providerId` | `string` | No | Pre-select a provider (for `provider-connect` screen) |
| `integrationId` | `string` | No | Pre-select an integration (for `integration-connect` screen) |
| `preview` | `boolean` | No | Enable preview mode |
| `onSuccess` | `(payload: SuccessPayload) => void` | No | Called when provider connection succeeds |
| `onIntegrationSuccess` | `(payload: IntegrationSuccessPayload) => void` | No | Called when OAuth integration succeeds |
| `onResourceSelection` | `(payload: ResourceSelectionPayload) => void` | No | Called when user selects resources |
| `onError` | `(payload: ErrorPayload) => void` | No | Called on errors |
| `onClose` | `() => void` | No | Called when widget is closed |
| `onEvent` | `(event: ConnectEvent) => void` | No | Called for every widget event |

**Throws:**

- `ConnectError` with type `'already_open'` if widget is already open
- `ConnectError` with type `'session_token_missing'` if sessionToken is empty

**Example:**

```typescript
connect.open({
  sessionToken: 'csess_...',
  screen: 'provider-selection',
  onSuccess: (payload) => {
    console.log('Provider connected:', payload.providerId)
    console.log('Connection ID:', payload.connectionId)
  },
  onError: (payload) => {
    console.error('Error:', payload.code, payload.message)
  },
  onClose: () => {
    console.log('Widget closed by user')
  }
})
```

---

### `close()`

Closes the Connect widget and removes the iframe.

```typescript
connect.close(): void
```

**Example:**

```typescript
// Close programmatically
connect.close()
```

---

### `isOpen`

Read-only property indicating whether the widget is currently open.

```typescript
connect.isOpen: boolean
```

**Example:**

```typescript
if (connect.isOpen) {
  console.log('Widget is open')
} else {
  console.log('Widget is closed')
}
```

---

## Connect Screens

The `screen` option controls which screen the widget displays on open:

| Screen | Description |
|--------|-------------|
| `'provider-selection'` | List of available LLM providers |
| `'integration-selection'` | List of configured OAuth integrations |
| `'connected-list'` | Show user's existing connections |
| `'provider-connect'` | Direct to provider connection flow (requires `providerId`) |
| `'integration-connect'` | Direct to integration connection flow (requires `integrationId`) |

**Example:**

```typescript
// Open to specific provider
connect.open({
  sessionToken: 'csess_...',
  screen: 'provider-connect',
  providerId: 'anthropic'
})

// Open to specific integration
connect.open({
  sessionToken: 'csess_...',
  screen: 'integration-connect',
  integrationId: 'int_slack_123'
})
```

---

## Event Callbacks

### `onSuccess`

Called when a provider connection is successfully created.

```typescript
onSuccess?: (payload: SuccessPayload) => void

interface SuccessPayload {
  providerId: string    // e.g., "openai", "anthropic"
  connectionId: string  // The created credential ID
}
```

**Example:**

```typescript
connect.open({
  sessionToken: 'csess_...',
  onSuccess: ({ providerId, connectionId }) => {
    // Save connectionId to your backend
    await fetch('/api/connections', {
      method: 'POST',
      body: JSON.stringify({ providerId, connectionId })
    })
  }
})
```

---

### `onIntegrationSuccess`

Called when an OAuth integration connection is successfully created.

```typescript
onIntegrationSuccess?: (payload: IntegrationSuccessPayload) => void

interface IntegrationSuccessPayload {
  integrationId: string  // The integration ID
  connectionId: string   // The created connection ID
}
```

**Example:**

```typescript
connect.open({
  sessionToken: 'csess_...',
  onIntegrationSuccess: ({ integrationId, connectionId }) => {
    console.log('Connected to integration:', integrationId)
  }
})
```

---

### `onResourceSelection`

Called when the user selects resources (e.g., Slack channels, GitHub repos).

```typescript
onResourceSelection?: (payload: ResourceSelectionPayload) => void

interface ResourceSelectionPayload {
  integrationId: string
  resources: Record<string, string[]>  // { channels: ['C001', 'C002'] }
}
```

**Example:**

```typescript
connect.open({
  sessionToken: 'csess_...',
  onResourceSelection: ({ integrationId, resources }) => {
    console.log('Selected resources for', integrationId)
    console.log('Channels:', resources.channels)
  }
})
```

---

### `onError`

Called when an error occurs in the widget.

```typescript
onError?: (payload: ErrorPayload) => void

interface ErrorPayload {
  code: ConnectErrorCode
  message: string
  providerId?: string  // Present for provider-related errors
}

type ConnectErrorCode =
  | 'session_invalid'
  | 'session_expired'
  | 'connection_failed'
  | 'integration_failed'
  | 'unknown_error'
```

**Example:**

```typescript
connect.open({
  sessionToken: 'csess_...',
  onError: ({ code, message, providerId }) => {
    console.error(`Error [${code}]:`, message)
    if (providerId) {
      console.error('Provider:', providerId)
    }
  }
})
```

---

### `onClose`

Called when the widget is closed (either by user or programmatically).

```typescript
onClose?: () => void
```

**Example:**

```typescript
connect.open({
  sessionToken: 'csess_...',
  onClose: () => {
    // Refresh connection list
    loadConnections()
  }
})
```

---

### `onEvent`

Called for every widget event. Useful for logging or analytics.

```typescript
onEvent?: (event: ConnectEvent) => void

type ConnectEvent =
  | { type: 'success'; payload: SuccessPayload }
  | { type: 'integration_success'; payload: IntegrationSuccessPayload }
  | { type: 'resource_selection'; payload: ResourceSelectionPayload }
  | { type: 'error'; payload: ErrorPayload }
  | { type: 'close' }
```

**Example:**

```typescript
connect.open({
  sessionToken: 'csess_...',
  onEvent: (event) => {
    // Log all events
    analytics.track('connect_event', {
      type: event.type,
      timestamp: Date.now()
    })
  }
})
```

---

## Error Handling

The SDK throws `ConnectError` for client-side errors:

```typescript
import { LLMVaultConnect, ConnectError } from '@llmvault/frontend'

const connect = new LLMVaultConnect()

try {
  connect.open({
    sessionToken: '' // Empty token
  })
} catch (error) {
  if (error instanceof ConnectError) {
    console.error('Type:', error.type)
    console.error('Message:', error.message)
    
    // Handle specific error types
    switch (error.type) {
      case 'session_token_missing':
        console.error('Please provide a session token')
        break
      case 'already_open':
        console.error('Widget is already open')
        break
      case 'iframe_blocked':
        console.error('Iframe was blocked by browser')
        break
    }
  }
}
```

### `ConnectError` Types

| Type | Description |
|------|-------------|
| `'session_token_missing'` | No session token provided |
| `'already_open'` | Widget is already open |
| `'iframe_blocked'` | Iframe was blocked by browser/security |

---

## Complete Example

```typescript
import { LLMVaultConnect, ConnectError } from '@llmvault/frontend'

class ConnectManager {
  private connect: LLMVaultConnect

  constructor() {
    // Initialize with custom theme
    this.connect = new LLMVaultConnect({
      baseURL: 'https://connect.llmvault.dev',
      theme: 'dark'
    })
  }

  async openConnectWidget() {
    // Fetch session token from your backend
    const response = await fetch('/api/connect-session', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        externalId: 'user_123',
        permissions: ['create', 'list']
      })
    })
    
    const { sessionToken } = await response.json()

    try {
      this.connect.open({
        sessionToken,
        screen: 'provider-selection',
        
        onSuccess: async ({ providerId, connectionId }) => {
          console.log('✅ Connected:', providerId)
          
          // Save to your backend
          await fetch('/api/connections', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ providerId, connectionId })
          })
          
          // Show success message
          showToast('Connected successfully!')
        },
        
        onIntegrationSuccess: ({ integrationId, connectionId }) => {
          console.log('✅ Integration connected:', integrationId)
        },
        
        onResourceSelection: ({ integrationId, resources }) => {
          console.log('📋 Resources selected:', resources)
        },
        
        onError: ({ code, message }) => {
          console.error('❌ Error:', code, message)
          showError(message)
        },
        
        onClose: () => {
          console.log('👋 Widget closed')
          // Refresh connections list
          this.loadConnections()
        },
        
        onEvent: (event) => {
          // Analytics
          analytics.track('connect', {
            event: event.type,
            timestamp: Date.now()
          })
        }
      })
    } catch (error) {
      if (error instanceof ConnectError) {
        console.error('Failed to open widget:', error.message)
      }
    }
  }

  closeWidget() {
    this.connect.close()
  }

  isOpen(): boolean {
    return this.connect.isOpen
  }

  private async loadConnections() {
    // Load user's connections
  }
}

// Usage
const manager = new ConnectManager()
document.getElementById('connect-btn')?.addEventListener('click', () => {
  manager.openConnectWidget()
})
```

---

## React Integration

```tsx
import { useEffect, useRef, useCallback } from 'react'
import { LLMVaultConnect, ConnectError } from '@llmvault/frontend'

export function useLLMVaultConnect() {
  const connectRef = useRef<LLMVaultConnect>()

  useEffect(() => {
    connectRef.current = new LLMVaultConnect({
      theme: 'system'
    })
    
    return () => {
      connectRef.current?.close()
    }
  }, [])

  const openConnect = useCallback(async (sessionToken: string) => {
    if (!connectRef.current) return

    try {
      connectRef.current.open({
        sessionToken,
        onSuccess: ({ providerId, connectionId }) => {
          console.log('Connected:', providerId, connectionId)
        },
        onError: ({ code, message }) => {
          console.error('Error:', code, message)
        }
      })
    } catch (error) {
      if (error instanceof ConnectError) {
        console.error('Connect error:', error.type)
      }
    }
  }, [])

  const closeConnect = useCallback(() => {
    connectRef.current?.close()
  }, [])

  return { openConnect, closeConnect, isOpen: connectRef.current?.isOpen }
}

// Usage in component
export function ConnectButton() {
  const { openConnect } = useLLMVaultConnect()

  const handleClick = async () => {
    // Fetch session token from your API
    const res = await fetch('/api/session')
    const { token } = await res.json()
    
    openConnect(token)
  }

  return <button onClick={handleClick}>Connect Provider</button>
}
```

---

## Vue Integration

```vue
<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { LLMVaultConnect, ConnectError } from '@llmvault/frontend'

const connect = ref<LLMVaultConnect>()

onMounted(() => {
  connect.value = new LLMVaultConnect({ theme: 'system' })
})

onUnmounted(() => {
  connect.value?.close()
})

async function openWidget() {
  if (!connect.value) return
  
  // Fetch session token from your API
  const res = await fetch('/api/session')
  const { token } = await res.json()
  
  try {
    connect.value.open({
      sessionToken: token,
      onSuccess: ({ providerId, connectionId }) => {
        console.log('Connected:', providerId)
      }
    })
  } catch (error) {
    if (error instanceof ConnectError) {
      console.error('Error:', error.message)
    }
  }
}
</script>

<template>
  <button @click="openWidget">Connect Provider</button>
</template>
```

---

## Type Definitions

```typescript
// Configuration
type ThemeOption = 'light' | 'dark' | 'system'

interface LLMVaultConnectConfig {
  baseURL?: string
  theme?: ThemeOption
}

// Screen options
type ConnectScreen =
  | 'provider-selection'
  | 'integration-selection'
  | 'connected-list'
  | 'provider-connect'
  | 'integration-connect'

// Open options
interface ConnectOpenOptions {
  sessionToken: string
  screen?: ConnectScreen
  providerId?: string
  integrationId?: string
  preview?: boolean
  onSuccess?: (payload: SuccessPayload) => void
  onIntegrationSuccess?: (payload: IntegrationSuccessPayload) => void
  onResourceSelection?: (payload: ResourceSelectionPayload) => void
  onError?: (payload: ErrorPayload) => void
  onClose?: () => void
  onEvent?: (event: ConnectEvent) => void
}

// Event payloads
interface SuccessPayload {
  providerId: string
  connectionId: string
}

interface IntegrationSuccessPayload {
  integrationId: string
  connectionId: string
}

interface ResourceSelectionPayload {
  integrationId: string
  resources: Record<string, string[]>
}

interface ErrorPayload {
  code: ConnectErrorCode
  message: string
  providerId?: string
}

type ConnectErrorCode =
  | 'session_invalid'
  | 'session_expired'
  | 'connection_failed'
  | 'integration_failed'
  | 'unknown_error'

// Event union type
type ConnectEvent =
  | { type: 'success'; payload: SuccessPayload }
  | { type: 'integration_success'; payload: IntegrationSuccessPayload }
  | { type: 'resource_selection'; payload: ResourceSelectionPayload }
  | { type: 'error'; payload: ErrorPayload }
  | { type: 'close' }

// Error types
type ConnectErrorType =
  | 'iframe_blocked'
  | 'session_token_missing'
  | 'already_open'

class ConnectError extends Error {
  type: ConnectErrorType
  constructor(message: string, type: ConnectErrorType)
}

// Main class
class LLMVaultConnect {
  constructor(config?: LLMVaultConnectConfig)
  open(options: ConnectOpenOptions): void
  close(): void
  get isOpen(): boolean
}
```

---

## Styling

The Connect widget renders as a full-screen iframe with the following default styles:

```css
/* These styles are applied automatically */
#llmvault-connect-iframe {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  border: none;
  z-index: 9999;
}
```

When the widget opens, `document.body.style.overflow` is set to `'hidden'` to prevent background scrolling. It is restored when the widget closes.

---

## Security

- The widget validates the origin of all postMessage communications
- Session tokens are short-lived and should be fetched fresh for each session
- Always fetch session tokens from your backend, never hardcode them
- The iframe is sandboxed to prevent access to parent page

## License

MIT License - see LICENSE file for details.
