---
title: Frontend SDK Reference
description: Complete reference for the @llmvault/frontend SDK including installation, configuration, events, and TypeScript types.
---

# Frontend SDK Reference

Complete reference for the `@llmvault/frontend` SDK, which provides a TypeScript-first interface for embedding the LLMVault Connect widget.

## Installation

```bash
npm install @llmvault/frontend
```

```bash
yarn add @llmvault/frontend
```

```bash
pnpm add @llmvault/frontend
```

## Quick Start

```typescript
import { LLMVaultConnect } from '@llmvault/frontend';

// Initialize
const connect = new LLMVaultConnect({
  theme: 'system',
});

// Open widget
connect.open({
  sessionToken: 'connect_session_xxx',
  onSuccess: (payload) => {
    console.log('Connected:', payload.providerId);
  },
});
```

## LLMVaultConnect Class

Main class for interacting with the Connect widget.

### Constructor

```typescript
new LLMVaultConnect(config?: LLMVaultConnectConfig)
```

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `config.baseURL` | `string` | No | `https://connect.llmvault.dev` | Widget URL |
| `config.theme` | `ThemeOption` | No | `'system'` | Default theme mode |

**Example:**

```typescript
// Production (default)
const connect = new LLMVaultConnect();

// Development
const connect = new LLMVaultConnect({
  baseURL: 'https://connect.dev.llmvault.dev',
  theme: 'light',
});
```

### Methods

#### `open(options: ConnectOpenOptions): void`

Opens the Connect widget in an iframe.

```typescript
connect.open({
  sessionToken: 'connect_session_xxx',
  screen: 'provider-selection',
  providerId: undefined,
  integrationId: undefined,
  preview: false,
  onSuccess: (payload) => {},
  onIntegrationSuccess: (payload) => {},
  onResourceSelection: (payload) => {},
  onError: (payload) => {},
  onClose: () => {},
  onEvent: (event) => {},
});
```

**Throws:**
- `ConnectError` with type `'already_open'` if widget is already open
- `ConnectError` with type `'session_token_missing'` if no session token provided

#### `close(): void`

Closes the widget and removes the iframe.

```typescript
connect.close();
```

#### `isOpen: boolean`

Read-only property indicating if the widget is currently open.

```typescript
if (connect.isOpen) {
  connect.close();
}
```

## Configuration Types

### `LLMVaultConnectConfig`

```typescript
interface LLMVaultConnectConfig {
  /** Widget base URL (default: https://connect.llmvault.dev) */
  baseURL?: string;
  
  /** Default theme mode */
  theme?: ThemeOption;
}
```

### `ThemeOption`

```typescript
type ThemeOption = 'light' | 'dark' | 'system';
```

| Value | Description |
|-------|-------------|
| `'light'` | Light mode (white background) |
| `'dark'` | Dark mode (dark background) |
| `'system'` | Follows user's OS preference |

## Open Options

### `ConnectOpenOptions`

```typescript
interface ConnectOpenOptions {
  /** Session token from your backend (required) */
  sessionToken: string;
  
  /** Initial screen to display */
  screen?: ConnectScreen;
  
  /** Provider ID for direct connection (when screen='provider-connect') */
  providerId?: string;
  
  /** Integration ID for direct connection (when screen='integration-connect') */
  integrationId?: string;
  
  /** Preview mode (bypasses session validation) */
  preview?: boolean;
  
  /** LLM provider connection success callback */
  onSuccess?: (payload: SuccessPayload) => void;
  
  /** Integration connection success callback */
  onIntegrationSuccess?: (payload: IntegrationSuccessPayload) => void;
  
  /** Resource selection callback */
  onResourceSelection?: (payload: ResourceSelectionPayload) => void;
  
  /** Error callback */
  onError?: (payload: ErrorPayload) => void;
  
  /** Widget close callback */
  onClose?: () => void;
  
  /** All events callback */
  onEvent?: (event: ConnectEvent) => void;
}
```

### `ConnectScreen`

```typescript
type ConnectScreen =
  | 'provider-selection'      // Choose LLM provider
  | 'integration-selection'   // Choose integration
  | 'connected-list'         // View existing connections
  | 'provider-connect'       // Direct to provider (needs providerId)
  | 'integration-connect';   // Direct to integration (needs integrationId)
```

## Event Types

### `ConnectEvent`

Union type of all possible events:

```typescript
type ConnectEvent =
  | { type: 'success'; payload: SuccessPayload }
  | { type: 'integration_success'; payload: IntegrationSuccessPayload }
  | { type: 'resource_selection'; payload: ResourceSelectionPayload }
  | { type: 'error'; payload: ErrorPayload }
  | { type: 'close' };
```

### `SuccessPayload`

Emitted when an LLM provider connection is successfully created.

```typescript
interface SuccessPayload {
  /** Provider identifier (e.g., 'openai', 'anthropic') */
  providerId: string;
  
  /** Connection ID for API calls */
  connectionId: string;
}
```

**Example:**

```typescript
connect.open({
  onSuccess: (payload) => {
    console.log(payload.providerId);  // 'openai'
    console.log(payload.connectionId); // '550e8400-e29b-41d4-a716-446655440000'
    
    // Store in your database
    await saveConnection(userId, payload);
  },
});
```

### `IntegrationSuccessPayload`

Emitted when an OAuth integration connection is successfully created.

```typescript
interface IntegrationSuccessPayload {
  /** Integration unique key */
  integrationId: string;
  
  /** Connection ID for API calls */
  connectionId: string;
}
```

**Example:**

```typescript
connect.open({
  onIntegrationSuccess: (payload) => {
    console.log(payload.integrationId); // 'slack-prod'
    console.log(payload.connectionId);  // '550e8400-e29b-41d4-a716-446655440000'
  },
});
```

### `ResourceSelectionPayload`

Emitted when a user selects resources during integration connection.

```typescript
interface ResourceSelectionPayload {
  /** Integration unique key */
  integrationId: string;
  
  /** Selected resources by type */
  resources: Record<string, string[]>;
}
```

**Example:**

```typescript
connect.open({
  onResourceSelection: (payload) => {
    console.log(payload.integrationId); // 'slack-prod'
    console.log(payload.resources);
    // {
    //   "channels": ["C123456", "C789012"],
    //   "users": ["U123456"]
    // }
  },
});
```

### `ErrorPayload`

Emitted when an error occurs in the widget.

```typescript
interface ErrorPayload {
  /** Error code */
  code: ConnectErrorCode;
  
  /** Human-readable error message */
  message: string;
  
  /** Provider ID (if applicable) */
  providerId?: string;
}
```

### `ConnectErrorCode`

```typescript
type ConnectErrorCode =
  | 'session_invalid'      // Session expired or invalid
  | 'session_expired'      // Session TTL reached
  | 'connection_failed'    // API key verification failed
  | 'integration_failed'   // OAuth authentication failed
  | 'unknown_error';       // Unexpected error
```

**Example:**

```typescript
connect.open({
  onError: (error) => {
    console.log(error.code);     // 'connection_failed'
    console.log(error.message);  // 'API key verification failed'
    console.log(error.providerId); // 'openai' (optional)
    
    // Handle specific errors
    switch (error.code) {
      case 'session_invalid':
        // Create new session
        break;
      case 'connection_failed':
        // Show key validation error
        break;
    }
  },
});
```

## Error Handling

### `ConnectError`

Error class thrown by the SDK for configuration issues.

```typescript
class ConnectError extends Error {
  type: ConnectErrorType;
  
  constructor(message: string, type: ConnectErrorType);
}
```

### `ConnectErrorType`

```typescript
type ConnectErrorType =
  | 'iframe_blocked'        // iframe was blocked by browser
  | 'session_token_missing' // No session token provided
  | 'already_open';         // Widget is already open
```

**Example:**

```typescript
import { LLMVaultConnect, ConnectError } from '@llmvault/frontend';

try {
  connect.open({ sessionToken: '' });
} catch (error) {
  if (error instanceof ConnectError) {
    switch (error.type) {
      case 'session_token_missing':
        console.error('Please provide a session token');
        break;
      case 'already_open':
        console.error('Widget is already open');
        break;
      case 'iframe_blocked':
        console.error('Please allow popups for this site');
        break;
    }
  }
}
```

## TypeScript Types

### Complete Type Definitions

```typescript
// Configuration
export type ThemeOption = 'light' | 'dark' | 'system';

export type ConnectScreen =
  | 'provider-selection'
  | 'integration-selection'
  | 'connected-list'
  | 'provider-connect'
  | 'integration-connect';

export type ConnectErrorCode =
  | 'session_invalid'
  | 'session_expired'
  | 'connection_failed'
  | 'integration_failed'
  | 'unknown_error';

// Events
export type ConnectEvent =
  | { type: 'success'; payload: SuccessPayload }
  | { type: 'integration_success'; payload: IntegrationSuccessPayload }
  | { type: 'resource_selection'; payload: ResourceSelectionPayload }
  | { type: 'error'; payload: ErrorPayload }
  | { type: 'close' };

// Payloads
export interface SuccessPayload {
  providerId: string;
  connectionId: string;
}

export interface IntegrationSuccessPayload {
  integrationId: string;
  connectionId: string;
}

export interface ResourceSelectionPayload {
  integrationId: string;
  resources: Record<string, string[]>;
}

export interface ErrorPayload {
  code: ConnectErrorCode;
  message: string;
  providerId?: string;
}

// Configuration interfaces
export interface LLMVaultConnectConfig {
  baseURL?: string;
  theme?: ThemeOption;
}

export interface ConnectOpenOptions {
  sessionToken: string;
  screen?: ConnectScreen;
  providerId?: string;
  integrationId?: string;
  preview?: boolean;
  onSuccess?: (payload: SuccessPayload) => void;
  onIntegrationSuccess?: (payload: IntegrationSuccessPayload) => void;
  onResourceSelection?: (payload: ResourceSelectionPayload) => void;
  onError?: (payload: ErrorPayload) => void;
  onClose?: () => void;
  onEvent?: (event: ConnectEvent) => void;
}

// Error types
export type ConnectErrorType =
  | 'iframe_blocked'
  | 'session_token_missing'
  | 'already_open';

export class ConnectError extends Error {
  type: ConnectErrorType;
  constructor(message: string, type: ConnectErrorType);
}

// Main class
export class LLMVaultConnect {
  constructor(config?: LLMVaultConnectConfig);
  open(options: ConnectOpenOptions): void;
  close(): void;
  get isOpen(): boolean;
}
```

## Usage Examples

### Basic Usage

```typescript
import { LLMVaultConnect } from '@llmvault/frontend';

const connect = new LLMVaultConnect();

function openConnect(sessionToken: string) {
  connect.open({
    sessionToken,
    onSuccess: ({ providerId, connectionId }) => {
      console.log(`Connected to ${providerId}: ${connectionId}`);
    },
    onError: ({ code, message }) => {
      console.error(`Error ${code}: ${message}`);
    },
    onClose: () => {
      console.log('Widget closed');
    },
  });
}
```

### React Hook

```typescript
import { useCallback, useRef } from 'react';
import { LLMVaultConnect, ConnectError } from '@llmvault/frontend';
import type { SuccessPayload, ErrorPayload } from '@llmvault/frontend';

export function useConnect() {
  const connectRef = useRef<LLMVaultConnect | null>(null);
  
  if (!connectRef.current) {
    connectRef.current = new LLMVaultConnect({
      theme: 'system',
    });
  }
  
  const open = useCallback((
    sessionToken: string,
    options?: {
      onSuccess?: (payload: SuccessPayload) => void;
      onError?: (payload: ErrorPayload) => void;
    }
  ) => {
    try {
      connectRef.current?.open({
        sessionToken,
        onSuccess: options?.onSuccess,
        onError: options?.onError,
      });
    } catch (error) {
      if (error instanceof ConnectError) {
        console.error('Connect error:', error.type, error.message);
      }
    }
  }, []);
  
  const close = useCallback(() => {
    connectRef.current?.close();
  }, []);
  
  return { open, close, isOpen: connectRef.current?.isOpen };
}
```

### Vue Composable

```typescript
import { ref, onUnmounted } from 'vue';
import { LLMVaultConnect, ConnectError } from '@llmvault/frontend';
import type { SuccessPayload } from '@llmvault/frontend';

export function useConnect() {
  const connect = new LLMVaultConnect();
  const isOpen = ref(false);
  
  function open(sessionToken: string) {
    try {
      connect.open({
        sessionToken,
        onSuccess: (payload: SuccessPayload) => {
          emit('success', payload);
        },
        onClose: () => {
          isOpen.value = false;
        },
      });
      isOpen.value = true;
    } catch (error) {
      if (error instanceof ConnectError) {
        console.error(error.message);
      }
    }
  }
  
  function close() {
    connect.close();
    isOpen.value = false;
  }
  
  onUnmounted(() => {
    close();
  });
  
  return { open, close, isOpen };
}
```

### Direct Provider Connection

```typescript
// Skip selection, go directly to OpenAI
connect.open({
  sessionToken: 'sess_xxx',
  screen: 'provider-connect',
  providerId: 'openai',
  onSuccess: (payload) => {
    console.log('OpenAI connected:', payload.connectionId);
  },
});
```

### Direct Integration Connection

```typescript
// Skip selection, go directly to Slack
connect.open({
  sessionToken: 'sess_xxx',
  screen: 'integration-connect',
  integrationId: 'slack-prod',
  onIntegrationSuccess: (payload) => {
    console.log('Slack connected:', payload.connectionId);
  },
  onResourceSelection: (payload) => {
    console.log('Channels selected:', payload.resources.channels);
  },
});
```

### Global Event Handler

```typescript
connect.open({
  sessionToken: 'sess_xxx',
  onEvent: (event) => {
    // Log all events for analytics
    analytics.track('connect_event', {
      type: event.type,
      timestamp: Date.now(),
    });
    
    switch (event.type) {
      case 'success':
        handleProviderSuccess(event.payload);
        break;
      case 'integration_success':
        handleIntegrationSuccess(event.payload);
        break;
      case 'resource_selection':
        handleResourceSelection(event.payload);
        break;
      case 'error':
        handleError(event.payload);
        break;
      case 'close':
        handleClose();
        break;
    }
  },
});
```

## Browser Compatibility

The SDK requires:

- **Modern browsers**: Chrome 80+, Firefox 75+, Safari 13+, Edge 80+
- **postMessage API**: Required for iframe communication
- **Promise support**: Native or polyfilled

## Bundle Size

- **Minified**: ~3 KB gzipped
- **Dependencies**: None (zero dependencies)

## Next Steps

- [Overview](./overview) — Connect widget overview
- [Embedding](./embedding) — Embedding guide
- [Theming](./theming) — Customize appearance
- [Sessions](./sessions) — Session management
