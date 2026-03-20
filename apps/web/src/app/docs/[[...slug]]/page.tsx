import {
  DocsBreadcrumb,
  DocsPageHeader,
  DocsPrevNext,
  DocsDivider,
} from "@/components/docs-nav";
import { DocsContent } from "./content";

type DocMeta = {
  title: string;
  description: string;
  breadcrumb: string[];
  section: string;
  prev?: { label: string; href: string };
  next?: { label: string; href: string };
  toc: { id: string; label: string; depth?: number }[];
};

const docPages: Record<string, DocMeta> = {
  // Getting Started
  introduction: {
    title: "Introduction",
    description:
      "LLMVault is a secure LLM API credential management platform that enables BYOK (Bring Your Own Key) functionality.",
    breadcrumb: ["Docs", "Getting Started", "Introduction"],
    section: "Getting Started",
    next: { label: "Quickstart", href: "/docs/quickstart" },
    toc: [
      { id: "what-is-llmvault", label: "What is LLMVault" },
      { id: "key-features", label: "Key features" },
      { id: "use-cases", label: "Use cases" },
      { id: "architecture-overview", label: "Architecture overview" },
    ],
  },
  quickstart: {
    title: "Quickstart",
    description:
      "Store your first credential, mint a token, and proxy a request in under five minutes.",
    breadcrumb: ["Docs", "Getting Started", "Quickstart"],
    section: "Getting Started",
    prev: { label: "Introduction", href: "/docs/introduction" },
    next: { label: "Installation", href: "/docs/installation" },
    toc: [
      { id: "get-your-api-key", label: "1. Get your API key" },
      { id: "store-a-credential", label: "2. Store a credential" },
      { id: "mint-a-scoped-token", label: "3. Mint a scoped token" },
      { id: "proxy-a-request", label: "4. Proxy a request" },
      { id: "next-steps", label: "Next steps" },
    ],
  },
  installation: {
    title: "Installation",
    description:
      "Install the LLMVault SDK for your language and configure your environment.",
    breadcrumb: ["Docs", "Getting Started", "Installation"],
    section: "Getting Started",
    prev: { label: "Quickstart", href: "/docs/quickstart" },
    next: { label: "Authentication", href: "/docs/authentication" },
    toc: [
      { id: "typescript", label: "TypeScript" },
      { id: "python", label: "Python" },
      { id: "go", label: "Go" },
      { id: "frontend-sdk", label: "Frontend SDK" },
      { id: "environment-variables", label: "Environment variables" },
    ],
  },
  authentication: {
    title: "Authentication",
    description:
      "Understand the authentication layers: organization API keys, proxy tokens, and Connect sessions.",
    breadcrumb: ["Docs", "Getting Started", "Authentication"],
    section: "Getting Started",
    prev: { label: "Installation", href: "/docs/installation" },
    next: { label: "Credentials", href: "/docs/credentials" },
    toc: [
      { id: "org-api-keys", label: "Organization API keys" },
      { id: "proxy-tokens", label: "Proxy tokens" },
      { id: "connect-sessions", label: "Connect sessions" },
      { id: "token-scoping", label: "Token scoping" },
    ],
  },

  // Core Concepts
  credentials: {
    title: "Credentials",
    description:
      "How LLMVault stores, encrypts, and manages your customers' LLM API keys.",
    breadcrumb: ["Docs", "Core Concepts", "Credentials"],
    section: "Core Concepts",
    prev: { label: "Authentication", href: "/docs/authentication" },
    next: { label: "Tokens", href: "/docs/tokens" },
    toc: [
      { id: "what-is-a-credential", label: "What is a credential" },
      { id: "envelope-encryption", label: "Envelope encryption" },
      { id: "auto-detection", label: "Provider auto-detection" },
      { id: "request-caps", label: "Request caps and refills" },
      { id: "lifecycle", label: "Credential lifecycle" },
    ],
  },
  tokens: {
    title: "Tokens",
    description:
      "Short-lived, scoped JWTs that grant sandboxed access to a single credential.",
    breadcrumb: ["Docs", "Core Concepts", "Tokens"],
    section: "Core Concepts",
    prev: { label: "Credentials", href: "/docs/credentials" },
    next: { label: "Proxy", href: "/docs/proxy" },
    toc: [
      { id: "token-anatomy", label: "Token anatomy" },
      { id: "ttl-and-expiry", label: "TTL and expiry" },
      { id: "scoping-rules", label: "Scoping rules" },
      { id: "mcp-endpoint", label: "MCP endpoint" },
    ],
  },
  proxy: {
    title: "Proxy",
    description:
      "The transparent pass-through that injects decrypted credentials into upstream LLM requests.",
    breadcrumb: ["Docs", "Core Concepts", "Proxy"],
    section: "Core Concepts",
    prev: { label: "Tokens", href: "/docs/tokens" },
    next: { label: "Providers", href: "/docs/providers" },
    toc: [
      { id: "how-it-works", label: "How it works" },
      { id: "streaming", label: "Streaming support" },
      { id: "caching", label: "Three-tier cache" },
      { id: "error-handling", label: "Error handling" },
    ],
  },
  providers: {
    title: "Providers",
    description:
      "Supported LLM providers and how LLMVault auto-detects them from base URLs.",
    breadcrumb: ["Docs", "Core Concepts", "Providers"],
    section: "Core Concepts",
    prev: { label: "Proxy", href: "/docs/proxy" },
    next: { label: "Identities", href: "/docs/identities" },
    toc: [
      { id: "supported-providers", label: "Supported providers" },
      { id: "auto-detection", label: "Auto-detection" },
      { id: "model-catalog", label: "Model catalog" },
      { id: "custom-providers", label: "Custom providers" },
    ],
  },
  identities: {
    title: "Identities",
    description:
      "Manage end-user identities with rate limits, metadata, and credential linking.",
    breadcrumb: ["Docs", "Core Concepts", "Identities"],
    section: "Core Concepts",
    prev: { label: "Providers", href: "/docs/providers" },
    next: { label: "Rate Limiting", href: "/docs/rate-limiting" },
    toc: [
      { id: "what-is-an-identity", label: "What is an identity" },
      { id: "external-ids", label: "External IDs" },
      { id: "metadata", label: "Metadata" },
      { id: "rate-limits", label: "Rate limits" },
    ],
  },
  "rate-limiting": {
    title: "Rate Limiting",
    description:
      "Understand the multiple layers of rate limiting in LLMVault.",
    breadcrumb: ["Docs", "Core Concepts", "Rate Limiting"],
    section: "Core Concepts",
    prev: { label: "Identities", href: "/docs/identities" },
    next: { label: "Connect Overview", href: "/docs/connect/overview" },
    toc: [
      { id: "org-rate-limits", label: "Organization rate limits" },
      { id: "identity-rate-limits", label: "Identity rate limits" },
      { id: "token-quotas", label: "Token quotas" },
      { id: "credential-caps", label: "Credential request caps" },
    ],
  },

  // Connect
  "connect/overview": {
    title: "Connect Overview",
    description:
      "Connect is an embeddable widget for secure LLM provider and OAuth integration connections.",
    breadcrumb: ["Docs", "Connect", "Overview"],
    section: "Connect",
    prev: { label: "Rate Limiting", href: "/docs/rate-limiting" },
    next: { label: "Embedding", href: "/docs/connect/embedding" },
    toc: [
      { id: "what-is-connect", label: "What is Connect" },
      { id: "features", label: "Features" },
      { id: "how-it-works", label: "How it works" },
      { id: "security", label: "Security" },
    ],
  },
  "connect/embedding": {
    title: "Embedding Connect",
    description:
      "Embed the Connect widget in your application using the Frontend SDK or iframe.",
    breadcrumb: ["Docs", "Connect", "Embedding"],
    section: "Connect",
    prev: { label: "Connect Overview", href: "/docs/connect/overview" },
    next: { label: "Theming", href: "/docs/connect/theming" },
    toc: [
      { id: "frontend-sdk", label: "Frontend SDK" },
      { id: "session-tokens", label: "Session tokens" },
      { id: "event-handling", label: "Event handling" },
      { id: "error-handling", label: "Error handling" },
    ],
  },
  "connect/theming": {
    title: "Theming",
    description:
      "Customize the appearance of the Connect widget to match your brand.",
    breadcrumb: ["Docs", "Connect", "Theming"],
    section: "Connect",
    prev: { label: "Embedding", href: "/docs/connect/embedding" },
    next: { label: "Sessions", href: "/docs/connect/sessions" },
    toc: [
      { id: "theme-modes", label: "Theme modes" },
      { id: "custom-colors", label: "Custom colors" },
      { id: "typography", label: "Typography" },
      { id: "border-radius", label: "Border radius" },
    ],
  },
  "connect/sessions": {
    title: "Connect Sessions",
    description:
      "Create and manage Connect sessions for secure widget authentication.",
    breadcrumb: ["Docs", "Connect", "Sessions"],
    section: "Connect",
    prev: { label: "Theming", href: "/docs/connect/theming" },
    next: { label: "Provider Connections", href: "/docs/connect/providers" },
    toc: [
      { id: "creating-sessions", label: "Creating sessions" },
      { id: "session-permissions", label: "Session permissions" },
      { id: "allowed-integrations", label: "Allowed integrations" },
      { id: "session-lifecycle", label: "Session lifecycle" },
    ],
  },
  "connect/providers": {
    title: "Provider Connections",
    description:
      "Enable users to connect LLM provider API keys through the Connect widget.",
    breadcrumb: ["Docs", "Connect", "Provider Connections"],
    section: "Connect",
    prev: { label: "Sessions", href: "/docs/connect/sessions" },
    next: { label: "Integration Connections", href: "/docs/connect/integrations" },
    toc: [
      { id: "supported-providers", label: "Supported providers" },
      { id: "connection-flow", label: "Connection flow" },
      { id: "validation", label: "API key validation" },
      { id: "managing-connections", label: "Managing connections" },
    ],
  },
  "connect/integrations": {
    title: "Integration Connections",
    description:
      "Enable users to connect OAuth integrations like Slack, GitHub, and Notion.",
    breadcrumb: ["Docs", "Connect", "Integration Connections"],
    section: "Connect",
    prev: { label: "Provider Connections", href: "/docs/connect/providers" },
    next: { label: "Frontend SDK", href: "/docs/connect/frontend-sdk" },
    toc: [
      { id: "oauth-flow", label: "OAuth flow" },
      { id: "resource-selection", label: "Resource selection" },
      { id: "scopes", label: "Scopes and permissions" },
      { id: "managing-integrations", label: "Managing integrations" },
    ],
  },
  "connect/frontend-sdk": {
    title: "Frontend SDK",
    description:
      "JavaScript SDK for embedding the Connect widget in your application.",
    breadcrumb: ["Docs", "Connect", "Frontend SDK"],
    section: "Connect",
    prev: { label: "Integration Connections", href: "/docs/connect/integrations" },
    next: { label: "Dashboard Overview", href: "/docs/dashboard/overview" },
    toc: [
      { id: "installation", label: "Installation" },
      { id: "configuration", label: "Configuration" },
      { id: "opening-the-widget", label: "Opening the widget" },
      { id: "event-callbacks", label: "Event callbacks" },
      { id: "typescript", label: "TypeScript support" },
    ],
  },

  // Dashboard
  "dashboard/overview": {
    title: "Dashboard Overview",
    description:
      "Navigate the LLMVault dashboard and understand the key metrics and features.",
    breadcrumb: ["Docs", "Dashboard", "Overview"],
    section: "Dashboard",
    prev: { label: "Frontend SDK", href: "/docs/connect/frontend-sdk" },
    next: { label: "Credentials", href: "/docs/dashboard/credentials" },
    toc: [
      { id: "getting-started", label: "Getting started" },
      { id: "dashboard-metrics", label: "Dashboard metrics" },
      { id: "navigation", label: "Navigation" },
      { id: "quick-actions", label: "Quick actions" },
    ],
  },
  "dashboard/credentials": {
    title: "Managing Credentials",
    description:
      "Create, view, and revoke LLM provider credentials in the dashboard.",
    breadcrumb: ["Docs", "Dashboard", "Credentials"],
    section: "Dashboard",
    prev: { label: "Dashboard Overview", href: "/docs/dashboard/overview" },
    next: { label: "Tokens", href: "/docs/dashboard/tokens" },
    toc: [
      { id: "creating-credentials", label: "Creating credentials" },
      { id: "viewing-credentials", label: "Viewing credentials" },
      { id: "credential-status", label: "Credential status" },
      { id: "revoking-credentials", label: "Revoking credentials" },
    ],
  },
  "dashboard/tokens": {
    title: "Managing Tokens",
    description:
      "Mint and manage scoped proxy tokens in the dashboard.",
    breadcrumb: ["Docs", "Dashboard", "Tokens"],
    section: "Dashboard",
    prev: { label: "Credentials", href: "/docs/dashboard/credentials" },
    next: { label: "API Keys", href: "/docs/dashboard/api-keys" },
    toc: [
      { id: "minting-tokens", label: "Minting tokens" },
      { id: "token-configuration", label: "Token configuration" },
      { id: "integration-access", label: "Integration access" },
      { id: "viewing-tokens", label: "Viewing tokens" },
      { id: "revoking-tokens", label: "Revoking tokens" },
    ],
  },
  "dashboard/api-keys": {
    title: "API Keys",
    description:
      "Create and manage organization API keys for programmatic access.",
    breadcrumb: ["Docs", "Dashboard", "API Keys"],
    section: "Dashboard",
    prev: { label: "Tokens", href: "/docs/dashboard/tokens" },
    next: { label: "Identities", href: "/docs/dashboard/identities" },
    toc: [
      { id: "creating-api-keys", label: "Creating API keys" },
      { id: "scopes", label: "Scopes and permissions" },
      { id: "expiration", label: "Expiration" },
      { id: "revoking-api-keys", label: "Revoking API keys" },
    ],
  },
  "dashboard/identities": {
    title: "Managing Identities",
    description:
      "View and manage end-user identities in the dashboard.",
    breadcrumb: ["Docs", "Dashboard", "Identities"],
    section: "Dashboard",
    prev: { label: "API Keys", href: "/docs/dashboard/api-keys" },
    next: { label: "Integrations", href: "/docs/dashboard/integrations" },
    toc: [
      { id: "viewing-identities", label: "Viewing identities" },
      { id: "identity-details", label: "Identity details" },
      { id: "linked-credentials", label: "Linked credentials" },
      { id: "rate-limit-configuration", label: "Rate limit configuration" },
    ],
  },
  "dashboard/integrations": {
    title: "Integrations",
    description:
      "Configure and manage third-party OAuth integrations.",
    breadcrumb: ["Docs", "Dashboard", "Integrations"],
    section: "Dashboard",
    prev: { label: "Identities", href: "/docs/dashboard/identities" },
    next: { label: "Audit Log", href: "/docs/dashboard/audit-log" },
    toc: [
      { id: "adding-integrations", label: "Adding integrations" },
      { id: "auth-modes", label: "Authentication modes" },
      { id: "credentials", label: "Credentials and secrets" },
      { id: "connections", label: "Managing connections" },
      { id: "webhooks", label: "Webhooks" },
    ],
  },
  "dashboard/audit-log": {
    title: "Audit Log",
    description:
      "Review all API and proxy requests in the audit log.",
    breadcrumb: ["Docs", "Dashboard", "Audit Log"],
    section: "Dashboard",
    prev: { label: "Integrations", href: "/docs/dashboard/integrations" },
    next: { label: "Team Management", href: "/docs/dashboard/team" },
    toc: [
      { id: "viewing-logs", label: "Viewing logs" },
      { id: "filtering", label: "Filtering and search" },
      { id: "log-details", label: "Log entry details" },
      { id: "retention", label: "Data retention" },
    ],
  },
  "dashboard/team": {
    title: "Team Management",
    description:
      "Invite team members and manage roles and permissions.",
    breadcrumb: ["Docs", "Dashboard", "Team Management"],
    section: "Dashboard",
    prev: { label: "Audit Log", href: "/docs/dashboard/audit-log" },
    next: { label: "Billing", href: "/docs/dashboard/billing" },
    toc: [
      { id: "inviting-members", label: "Inviting members" },
      { id: "roles", label: "Roles and permissions" },
      { id: "managing-members", label: "Managing members" },
      { id: "organization-switching", label: "Organization switching" },
    ],
  },
  "dashboard/billing": {
    title: "Billing",
    description:
      "Manage your subscription and view invoice history.",
    breadcrumb: ["Docs", "Dashboard", "Billing"],
    section: "Dashboard",
    prev: { label: "Team Management", href: "/docs/dashboard/team" },
    next: { label: "API Overview", href: "/docs/api/overview" },
    toc: [
      { id: "plans", label: "Plans and pricing" },
      { id: "usage-limits", label: "Usage limits" },
      { id: "upgrading", label: "Upgrading" },
      { id: "invoices", label: "Invoice history" },
    ],
  },

  // API Reference
  "api/overview": {
    title: "API Overview",
    description:
      "Overview of the LLMVault API architecture and conventions.",
    breadcrumb: ["Docs", "API Reference", "Overview"],
    section: "API Reference",
    prev: { label: "Billing", href: "/docs/dashboard/billing" },
    next: { label: "Authentication", href: "/docs/api/authentication" },
    toc: [
      { id: "base-urls", label: "Base URLs" },
      { id: "authentication", label: "Authentication methods" },
      { id: "pagination", label: "Pagination" },
      { id: "rate-limiting", label: "Rate limiting" },
      { id: "errors", label: "Error handling" },
    ],
  },
  "api/authentication": {
    title: "API Authentication",
    description:
      "Detailed guide to API authentication methods.",
    breadcrumb: ["Docs", "API Reference", "Authentication"],
    section: "API Reference",
    prev: { label: "API Overview", href: "/docs/api/overview" },
    next: { label: "Credentials API", href: "/docs/api/credentials" },
    toc: [
      { id: "logto-jwt", label: "Logto JWT" },
      { id: "api-keys", label: "API Keys" },
      { id: "proxy-tokens", label: "Proxy Tokens" },
      { id: "connect-sessions", label: "Connect Sessions" },
    ],
  },
  "api/credentials": {
    title: "Credentials API",
    description:
      "Store, retrieve, and manage encrypted LLM provider credentials.",
    breadcrumb: ["Docs", "API Reference", "Credentials"],
    section: "API Reference",
    prev: { label: "Authentication", href: "/docs/api/authentication" },
    next: { label: "Tokens API", href: "/docs/api/tokens" },
    toc: [
      { id: "post-credentials", label: "POST /v1/credentials" },
      { id: "get-credentials", label: "GET /v1/credentials" },
      { id: "get-credential", label: "GET /v1/credentials/:id" },
      { id: "delete-credential", label: "DELETE /v1/credentials/:id" },
    ],
  },
  "api/tokens": {
    title: "Tokens API",
    description: "Mint and manage scoped proxy tokens bound to credentials.",
    breadcrumb: ["Docs", "API Reference", "Tokens"],
    section: "API Reference",
    prev: { label: "Credentials API", href: "/docs/api/credentials" },
    next: { label: "Identities API", href: "/docs/api/identities" },
    toc: [
      { id: "post-tokens", label: "POST /v1/tokens" },
      { id: "get-tokens", label: "GET /v1/tokens" },
      { id: "revoke-token", label: "DELETE /v1/tokens/:jti" },
    ],
  },
  "api/identities": {
    title: "Identities API",
    description: "Create and manage end-user identities.",
    breadcrumb: ["Docs", "API Reference", "Identities"],
    section: "API Reference",
    prev: { label: "Tokens API", href: "/docs/api/tokens" },
    next: { label: "API Keys", href: "/docs/api/api-keys" },
    toc: [
      { id: "post-identities", label: "POST /v1/identities" },
      { id: "get-identities", label: "GET /v1/identities" },
      { id: "get-identity", label: "GET /v1/identities/:id" },
      { id: "put-identity", label: "PUT /v1/identities/:id" },
      { id: "delete-identity", label: "DELETE /v1/identities/:id" },
    ],
  },
  "api/api-keys": {
    title: "API Keys",
    description: "Create and manage organization API keys.",
    breadcrumb: ["Docs", "API Reference", "API Keys"],
    section: "API Reference",
    prev: { label: "Identities API", href: "/docs/api/identities" },
    next: { label: "Integrations API", href: "/docs/api/integrations" },
    toc: [
      { id: "post-api-keys", label: "POST /v1/api-keys" },
      { id: "get-api-keys", label: "GET /v1/api-keys" },
      { id: "delete-api-key", label: "DELETE /v1/api-keys/:id" },
    ],
  },
  "api/integrations": {
    title: "Integrations API",
    description: "Configure OAuth integrations.",
    breadcrumb: ["Docs", "API Reference", "Integrations"],
    section: "API Reference",
    prev: { label: "API Keys", href: "/docs/api/api-keys" },
    next: { label: "Connections API", href: "/docs/api/connections" },
    toc: [
      { id: "post-integrations", label: "POST /v1/integrations" },
      { id: "get-integrations", label: "GET /v1/integrations" },
      { id: "get-integration", label: "GET /v1/integrations/:id" },
      { id: "put-integration", label: "PUT /v1/integrations/:id" },
      { id: "delete-integration", label: "DELETE /v1/integrations/:id" },
    ],
  },
  "api/connections": {
    title: "Connections API",
    description: "Manage OAuth connections.",
    breadcrumb: ["Docs", "API Reference", "Connections"],
    section: "API Reference",
    prev: { label: "Integrations API", href: "/docs/api/integrations" },
    next: { label: "Connect Sessions", href: "/docs/api/connect-sessions" },
    toc: [
      { id: "get-available-scopes", label: "GET /v1/connections/available-scopes" },
      { id: "get-connection", label: "GET /v1/connections/:id" },
      { id: "delete-connection", label: "DELETE /v1/connections/:id" },
      { id: "post-token", label: "POST /v1/connections/:id/token" },
    ],
  },
  "api/connect-sessions": {
    title: "Connect Sessions",
    description: "Create Connect widget sessions.",
    breadcrumb: ["Docs", "API Reference", "Connect Sessions"],
    section: "API Reference",
    prev: { label: "Connections API", href: "/docs/api/connections" },
    next: { label: "Widget API", href: "/docs/api/widget" },
    toc: [
      { id: "post-sessions", label: "POST /v1/connect/sessions" },
    ],
  },
  "api/widget": {
    title: "Widget API",
    description: "Internal API for the Connect widget.",
    breadcrumb: ["Docs", "API Reference", "Widget API"],
    section: "API Reference",
    prev: { label: "Connect Sessions", href: "/docs/api/connect-sessions" },
    next: { label: "Proxy API", href: "/docs/api/proxy" },
    toc: [
      { id: "get-session", label: "GET /v1/widget/session" },
      { id: "get-providers", label: "GET /v1/widget/providers" },
      { id: "get-integrations", label: "GET /v1/widget/integrations" },
      { id: "post-connection", label: "POST /v1/widget/connections" },
      { id: "post-integration-connect", label: "POST /v1/widget/integrations/:id/connect-session" },
    ],
  },
  "api/proxy": {
    title: "Proxy API",
    description:
      "Forward requests to LLM providers through the encrypted proxy.",
    breadcrumb: ["Docs", "API Reference", "Proxy"],
    section: "API Reference",
    prev: { label: "Widget API", href: "/docs/api/widget" },
    next: { label: "Providers API", href: "/docs/api/providers" },
    toc: [
      { id: "proxy-endpoint", label: "Proxy endpoint" },
      { id: "auth-header", label: "Authentication" },
      { id: "streaming", label: "Streaming" },
    ],
  },
  "api/providers": {
    title: "Providers API",
    description: "List LLM providers and their models.",
    breadcrumb: ["Docs", "API Reference", "Providers"],
    section: "API Reference",
    prev: { label: "Proxy API", href: "/docs/api/proxy" },
    next: { label: "Audit API", href: "/docs/api/audit" },
    toc: [
      { id: "get-providers", label: "GET /v1/providers" },
      { id: "get-provider", label: "GET /v1/providers/:id" },
      { id: "get-models", label: "GET /v1/providers/:id/models" },
    ],
  },
  "api/audit": {
    title: "Audit API",
    description: "Query audit log entries.",
    breadcrumb: ["Docs", "API Reference", "Audit"],
    section: "API Reference",
    prev: { label: "Providers API", href: "/docs/api/providers" },
    next: { label: "MCP Server", href: "/docs/api/mcp" },
    toc: [
      { id: "get-audit", label: "GET /v1/audit" },
    ],
  },
  "api/mcp": {
    title: "MCP Server",
    description: "Model Context Protocol server endpoints.",
    breadcrumb: ["Docs", "API Reference", "MCP Server"],
    section: "API Reference",
    prev: { label: "Audit API", href: "/docs/api/audit" },
    next: { label: "TypeScript SDK", href: "/docs/sdk/typescript" },
    toc: [
      { id: "overview", label: "Overview" },
      { id: "streamable-http", label: "Streamable HTTP transport" },
      { id: "sse-transport", label: "SSE transport" },
      { id: "scopes", label: "MCP scopes" },
    ],
  },

  // SDKs
  "sdk/typescript": {
    title: "TypeScript SDK",
    description:
      "Full reference for the official LLMVault TypeScript client library.",
    breadcrumb: ["Docs", "SDKs", "TypeScript"],
    section: "SDKs",
    prev: { label: "MCP Server", href: "/docs/api/mcp" },
    next: { label: "Python SDK", href: "/docs/sdk/python" },
    toc: [
      { id: "installation", label: "Installation" },
      { id: "client-setup", label: "Client setup" },
      { id: "credentials", label: "Credentials" },
      { id: "tokens", label: "Tokens" },
      { id: "identities", label: "Identities" },
      { id: "integrations", label: "Integrations" },
      { id: "connections", label: "Connections" },
      { id: "usage", label: "Usage" },
      { id: "audit", label: "Audit" },
    ],
  },
  "sdk/python": {
    title: "Python SDK",
    description: "Full reference for the official LLMVault Python client library.",
    breadcrumb: ["Docs", "SDKs", "Python"],
    section: "SDKs",
    prev: { label: "TypeScript SDK", href: "/docs/sdk/typescript" },
    next: { label: "Go SDK", href: "/docs/sdk/go" },
    toc: [
      { id: "installation", label: "Installation" },
      { id: "client-setup", label: "Client setup" },
      { id: "credentials", label: "Credentials" },
      { id: "tokens", label: "Tokens" },
      { id: "proxy", label: "Proxy" },
    ],
  },
  "sdk/go": {
    title: "Go SDK",
    description: "Full reference for the official LLMVault Go client library.",
    breadcrumb: ["Docs", "SDKs", "Go"],
    section: "SDKs",
    prev: { label: "Python SDK", href: "/docs/sdk/python" },
    next: { label: "Frontend SDK", href: "/docs/sdk/frontend" },
    toc: [
      { id: "installation", label: "Installation" },
      { id: "client-setup", label: "Client setup" },
      { id: "credentials", label: "Credentials" },
      { id: "tokens", label: "Tokens" },
      { id: "proxy", label: "Proxy" },
    ],
  },
  "sdk/frontend": {
    title: "Frontend SDK",
    description: "JavaScript SDK for embedding the Connect widget.",
    breadcrumb: ["Docs", "SDKs", "Frontend"],
    section: "SDKs",
    prev: { label: "Go SDK", href: "/docs/sdk/go" },
    next: { label: "Security Overview", href: "/docs/security/overview" },
    toc: [
      { id: "installation", label: "Installation" },
      { id: "configuration", label: "Configuration" },
      { id: "opening-widget", label: "Opening the widget" },
      { id: "event-handling", label: "Event handling" },
      { id: "typescript", label: "TypeScript support" },
    ],
  },

  // Security
  "security/overview": {
    title: "Security Overview",
    description:
      "A complete overview of how LLMVault protects your customers' API keys at every layer.",
    breadcrumb: ["Docs", "Security", "Overview"],
    section: "Security",
    prev: { label: "Frontend SDK", href: "/docs/sdk/frontend" },
    next: { label: "Encryption", href: "/docs/security/encryption" },
    toc: [
      { id: "threat-model", label: "Threat model" },
      { id: "security-layers", label: "Security layers" },
      { id: "certifications", label: "Certifications" },
    ],
  },
  "security/encryption": {
    title: "Encryption",
    description:
      "Details of LLMVault's envelope encryption and key management.",
    breadcrumb: ["Docs", "Security", "Encryption"],
    section: "Security",
    prev: { label: "Security Overview", href: "/docs/security/overview" },
    next: { label: "Token Scoping", href: "/docs/security/token-scoping" },
    toc: [
      { id: "envelope-encryption", label: "Envelope encryption" },
      { id: "kms", label: "Key management service" },
      { id: "dek-rotation", label: "DEK rotation" },
      { id: "memory-protection", label: "Memory protection" },
    ],
  },
  "security/token-scoping": {
    title: "Token Scoping",
    description:
      "How scoped tokens limit access to specific resources and actions.",
    breadcrumb: ["Docs", "Security", "Token Scoping"],
    section: "Security",
    prev: { label: "Encryption", href: "/docs/security/encryption" },
    next: { label: "Audit Logging", href: "/docs/security/audit-logging" },
    toc: [
      { id: "scope-structure", label: "Scope structure" },
      { id: "actions", label: "Actions" },
      { id: "resources", label: "Resources" },
      { id: "mcp-scopes", label: "MCP scopes" },
    ],
  },
  "security/audit-logging": {
    title: "Audit Logging",
    description:
      "Complete audit trail of all API and proxy requests.",
    breadcrumb: ["Docs", "Security", "Audit Logging"],
    section: "Security",
    prev: { label: "Token Scoping", href: "/docs/security/token-scoping" },
    next: { label: "Compliance", href: "/docs/security/compliance" },
    toc: [
      { id: "logged-events", label: "Logged events" },
      { id: "log-retention", label: "Log retention" },
      { id: "querying-logs", label: "Querying logs" },
    ],
  },
  "security/compliance": {
    title: "Compliance",
    description:
      "Compliance certifications and security standards.",
    breadcrumb: ["Docs", "Security", "Compliance"],
    section: "Security",
    prev: { label: "Audit Logging", href: "/docs/security/audit-logging" },
    next: { label: "Self-Hosting Overview", href: "/docs/self-hosting/overview" },
    toc: [
      { id: "soc2", label: "SOC 2" },
      { id: "gdpr", label: "GDPR" },
      { id: "hipaa", label: "HIPAA" },
    ],
  },

  // Self-Hosting
  "self-hosting/overview": {
    title: "Self-Hosting Overview",
    description:
      "Deploy LLMVault in your own infrastructure.",
    breadcrumb: ["Docs", "Self-Hosting", "Overview"],
    section: "Self-Hosting",
    prev: { label: "Compliance", href: "/docs/security/compliance" },
    next: { label: "Docker Compose", href: "/docs/self-hosting/docker-compose" },
    toc: [
      { id: "requirements", label: "Requirements" },
      { id: "components", label: "Components" },
      { id: "architecture", label: "Architecture" },
    ],
  },
  "self-hosting/docker-compose": {
    title: "Docker Compose",
    description:
      "Deploy LLMVault using Docker Compose.",
    breadcrumb: ["Docs", "Self-Hosting", "Docker Compose"],
    section: "Self-Hosting",
    prev: { label: "Self-Hosting Overview", href: "/docs/self-hosting/overview" },
    next: { label: "Kubernetes", href: "/docs/self-hosting/kubernetes" },
    toc: [
      { id: "prerequisites", label: "Prerequisites" },
      { id: "configuration", label: "Configuration" },
      { id: "deployment", label: "Deployment" },
      { id: "upgrading", label: "Upgrading" },
    ],
  },
  "self-hosting/kubernetes": {
    title: "Kubernetes",
    description:
      "Deploy LLMVault on Kubernetes.",
    breadcrumb: ["Docs", "Self-Hosting", "Kubernetes"],
    section: "Self-Hosting",
    prev: { label: "Docker Compose", href: "/docs/self-hosting/docker-compose" },
    next: { label: "Configuration", href: "/docs/self-hosting/configuration" },
    toc: [
      { id: "prerequisites", label: "Prerequisites" },
      { id: "helm-chart", label: "Helm chart" },
      { id: "manifests", label: "Kubernetes manifests" },
      { id: "ingress", label: "Ingress configuration" },
    ],
  },
  "self-hosting/configuration": {
    title: "Configuration",
    description:
      "Configure LLMVault for self-hosted deployments.",
    breadcrumb: ["Docs", "Self-Hosting", "Configuration"],
    section: "Self-Hosting",
    prev: { label: "Kubernetes", href: "/docs/self-hosting/kubernetes" },
    next: { label: "Environment Variables", href: "/docs/self-hosting/environment" },
    toc: [
      { id: "database", label: "Database" },
      { id: "redis", label: "Redis" },
      { id: "kms", label: "KMS provider" },
      { id: "logto", label: "Logto" },
    ],
  },
  "self-hosting/environment": {
    title: "Environment Variables",
    description:
      "Complete reference of all environment variables.",
    breadcrumb: ["Docs", "Self-Hosting", "Environment Variables"],
    section: "Self-Hosting",
    prev: { label: "Configuration", href: "/docs/self-hosting/configuration" },
    toc: [
      { id: "required", label: "Required variables" },
      { id: "database", label: "Database" },
      { id: "redis", label: "Redis" },
      { id: "encryption", label: "Encryption" },
      { id: "oauth", label: "OAuth" },
      { id: "optional", label: "Optional variables" },
    ],
  },
};

export default async function DocsPage({
  params,
}: {
  params: Promise<{ slug?: string[] }>;
}) {
  const { slug } = await params;
  const path = slug?.join("/") || "quickstart";
  const meta = docPages[path];

  if (!meta) {
    return (
      <div className="flex flex-col gap-8">
        <DocsPageHeader
          title="Not Found"
          description="This documentation page doesn't exist yet."
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-8">
      <DocsBreadcrumb items={meta.breadcrumb} />
      <DocsPageHeader title={meta.title} description={meta.description} />
      <DocsDivider />
      <DocsContent slug={path} toc={meta.toc} />
      <DocsPrevNext prev={meta.prev} next={meta.next} />
    </div>
  );
}
