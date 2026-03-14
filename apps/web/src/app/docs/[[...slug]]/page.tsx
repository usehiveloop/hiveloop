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
  quickstart: {
    title: "Quickstart",
    description:
      "Store your first credential, mint a token, and proxy a request in under five minutes.",
    breadcrumb: ["Docs", "Getting Started", "Quickstart"],
    section: "Getting Started",
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
      { id: "environment-variables", label: "Environment variables" },
    ],
  },
  authentication: {
    title: "Authentication",
    description:
      "Understand the two authentication layers: organization API keys and scoped proxy tokens.",
    breadcrumb: ["Docs", "Getting Started", "Authentication"],
    section: "Getting Started",
    prev: { label: "Installation", href: "/docs/installation" },
    next: { label: "Credentials", href: "/docs/credentials" },
    toc: [
      { id: "org-api-keys", label: "Organization API keys" },
      { id: "proxy-tokens", label: "Proxy tokens" },
      { id: "token-scoping", label: "Token scoping" },
    ],
  },
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
    ],
  },
  providers: {
    title: "Providers",
    description:
      "Supported LLM providers and how LLMVault auto-detects them from base URLs.",
    breadcrumb: ["Docs", "Core Concepts", "Providers"],
    section: "Core Concepts",
    prev: { label: "Proxy", href: "/docs/proxy" },
    next: { label: "Credentials API", href: "/docs/api/credentials" },
    toc: [
      { id: "supported-providers", label: "Supported providers" },
      { id: "auto-detection", label: "Auto-detection" },
      { id: "model-catalog", label: "Model catalog" },
    ],
  },
  "api/credentials": {
    title: "Credentials API",
    description:
      "Store, retrieve, and manage encrypted LLM provider credentials.",
    breadcrumb: ["Docs", "API Reference", "Credentials"],
    section: "API Reference",
    prev: { label: "Providers", href: "/docs/providers" },
    next: { label: "Tokens API", href: "/docs/api/tokens" },
    toc: [
      { id: "post-credentials", label: "POST /v1/credentials" },
      { id: "get-credentials", label: "GET /v1/credentials" },
      { id: "delete-credential", label: "DELETE /v1/credentials/:id" },
    ],
  },
  "api/tokens": {
    title: "Tokens API",
    description: "Mint and manage scoped proxy tokens bound to credentials.",
    breadcrumb: ["Docs", "API Reference", "Tokens"],
    section: "API Reference",
    prev: { label: "Credentials API", href: "/docs/api/credentials" },
    next: { label: "Proxy API", href: "/docs/api/proxy" },
    toc: [
      { id: "post-tokens", label: "POST /v1/tokens" },
      { id: "get-tokens", label: "GET /v1/tokens" },
      { id: "revoke-token", label: "DELETE /v1/tokens/:id" },
    ],
  },
  "api/proxy": {
    title: "Proxy API",
    description:
      "Forward requests to LLM providers through the encrypted proxy.",
    breadcrumb: ["Docs", "API Reference", "Proxy"],
    section: "API Reference",
    prev: { label: "Tokens API", href: "/docs/api/tokens" },
    next: { label: "Architecture", href: "/docs/architecture" },
    toc: [
      { id: "proxy-endpoint", label: "Proxy endpoint" },
      { id: "auth-header", label: "Authentication" },
      { id: "streaming", label: "Streaming" },
    ],
  },
  architecture: {
    title: "Architecture",
    description:
      "A deep-dive into LLMVault's system design, data flow, and deployment topology.",
    breadcrumb: ["Docs", "Infrastructure", "Architecture"],
    section: "Infrastructure",
    prev: { label: "Proxy API", href: "/docs/api/proxy" },
    next: { label: "Security", href: "/docs/security" },
    toc: [
      { id: "system-overview", label: "System overview" },
      { id: "data-flow", label: "Data flow" },
      { id: "deployment", label: "Deployment topology" },
    ],
  },
  security: {
    title: "Security",
    description:
      "A complete overview of how LLMVault protects your customers' API keys at every layer.",
    breadcrumb: ["Docs", "Infrastructure", "Security"],
    section: "Infrastructure",
    prev: { label: "Architecture", href: "/docs/architecture" },
    next: { label: "Self-Hosting", href: "/docs/self-hosting" },
    toc: [
      { id: "threat-model", label: "Threat Model" },
      { id: "envelope-encryption", label: "Envelope Encryption" },
      { id: "memory-protection", label: "Memory Protection" },
      { id: "cache-layer-security", label: "Cache Layer Security" },
      { id: "multi-tenant-isolation", label: "Multi-Tenant Isolation" },
      { id: "container-security", label: "Container Security" },
      { id: "compliance", label: "Compliance" },
    ],
  },
  "self-hosting": {
    title: "Self-Hosting",
    description:
      "Deploy LLMVault in your own infrastructure with Docker Compose or Kubernetes.",
    breadcrumb: ["Docs", "Infrastructure", "Self-Hosting"],
    section: "Infrastructure",
    prev: { label: "Security", href: "/docs/security" },
    next: { label: "TypeScript SDK", href: "/docs/sdk/typescript" },
    toc: [
      { id: "requirements", label: "Requirements" },
      { id: "docker-compose", label: "Docker Compose" },
      { id: "kubernetes", label: "Kubernetes" },
      { id: "configuration", label: "Configuration" },
    ],
  },
  "sdk/typescript": {
    title: "TypeScript SDK",
    description:
      "Full reference for the official LLMVault TypeScript client library.",
    breadcrumb: ["Docs", "SDKs", "TypeScript"],
    section: "SDKs",
    prev: { label: "Self-Hosting", href: "/docs/self-hosting" },
    next: { label: "Python SDK", href: "/docs/sdk/python" },
    toc: [
      { id: "installation", label: "Installation" },
      { id: "client-setup", label: "Client setup" },
      { id: "credentials", label: "Credentials" },
      { id: "tokens", label: "Tokens" },
      { id: "proxy", label: "Proxy" },
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
    toc: [
      { id: "installation", label: "Installation" },
      { id: "client-setup", label: "Client setup" },
      { id: "credentials", label: "Credentials" },
      { id: "tokens", label: "Tokens" },
      { id: "proxy", label: "Proxy" },
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
