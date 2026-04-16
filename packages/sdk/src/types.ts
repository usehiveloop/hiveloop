import type { components } from "./generated/schema.js";

export interface ZiraLoopConfig {
  apiKey: string;
  baseUrl?: string;
}

// Re-export schema types with friendly aliases
type Schemas = components["schemas"];

export type ApiKeyResponse = Schemas["apiKeyResponse"];
export type CreateAPIKeyRequest = Schemas["createAPIKeyRequest"];
export type CreateAPIKeyResponse = Schemas["createAPIKeyResponse"];

export type CredentialResponse = Schemas["credentialResponse"];
export type CreateCredentialRequest = Schemas["createCredentialRequest"];

export type MintTokenRequest = Schemas["mintTokenRequest"];
export type MintTokenResponse = Schemas["mintTokenResponse"];
export type TokenListItem = Schemas["tokenListItem"];
export type PaginatedTokens = Schemas["paginatedResponse-tokenListItem"];
export type TokenScope = Schemas["github_com_ziraloop_ziraloop_internal_mcp.TokenScope"];

// Available scopes for token minting (used by scope selection UI)
export interface AvailableScopeAction {
  key: string;
  display_name: string;
  description: string;
  resource_type?: string;
}

export interface AvailableScopeResourceItem {
  id: string;
  name: string;
}

export interface AvailableScopeResource {
  display_name: string;
  selected: AvailableScopeResourceItem[];
}

export interface AvailableScopeConnection {
  connection_id: string;
  integration_id: string;
  provider: string;
  display_name: string;
  actions: AvailableScopeAction[];
  resources?: Record<string, AvailableScopeResource>;
}



export type IntegrationResponse = Schemas["integrationResponse"];
export type CreateIntegrationRequest = Schemas["createIntegrationRequest"];
export type UpdateIntegrationRequest = Schemas["updateIntegrationRequest"];
export type NangoCredentials = Schemas["github_com_ziraloop_ziraloop_internal_nango.Credentials"];

export type IntegConnResponse = Schemas["integConnResponse"];
export type IntegConnCreateRequest = Schemas["integConnCreateRequest"];

export type UsageResponse = Schemas["usageResponse"];
export type AuditEntryResponse = Schemas["auditEntryResponse"];

export type OrgResponse = Schemas["orgResponse"];

export type ProviderSummary = Schemas["providerSummary"];
export type ProviderDetail = Schemas["providerDetail"];
export type ModelSummary = Schemas["modelSummary"];

export type PaginatedApiKeys = Schemas["paginatedResponse-apiKeyResponse"];
export type PaginatedCredentials = Schemas["paginatedResponse-credentialResponse"];
export type PaginatedIdentities = Schemas["paginatedResponse-identityResponse"];
export type PaginatedAuditEntries = Schemas["paginatedResponse-auditEntryResponse"];
export type PaginatedIntegrations = Schemas["paginatedResponse-integrationResponse"];
export type PaginatedIntegConns = Schemas["paginatedResponse-integConnResponse"];

export type ErrorResponse = Schemas["errorResponse"];
export type JSON = Schemas["JSON"];

// Org
export type CreateOrgRequest = Schemas["createOrgRequest"];

// Generations
export type GenerationResponse = Schemas["generationResponse"];
export type PaginatedGenerations = Schemas["paginatedResponse-generationResponse"];

// Reporting
export type ReportRow = Schemas["reportRow"];

// Custom Domains
export type CreateDomainRequest = Schemas["createDomainRequest"];
export type CreateDomainResponse = Schemas["createDomainResponse"];
export type VerifyDomainResponse = Schemas["verifyDomainResponse"];
export type DnsRecord = Schemas["dnsRecord"];

// Connect Sessions
export type ConnectSessionListItem = Schemas["connectSessionListItem"];
export type PaginatedConnectSessions = Schemas["paginatedResponse-connectSessionListItem"];

// Catalog
export type IntegrationSummary = Schemas["integrationSummary"];
export type IntegrationDetail = Schemas["integrationDetail"];
export type ActionSummary = Schemas["actionSummary"];

// Setup
export type SetupRequest = Schemas["setupRequest"];
export type SetupResponse = Schemas["setupResponse"];

// Agents
export type AgentResponse = Schemas["agentResponse"];
export type CreateAgentRequest = Schemas["createAgentRequest"];
export type UpdateAgentRequest = Schemas["updateAgentRequest"];
export type PaginatedAgents = Schemas["paginatedResponse-agentResponse"];

// Sandbox Templates
export type SandboxTemplateResponse = Schemas["sandboxTemplateResponse"];
export type CreateSandboxTemplateRequest = Schemas["createSandboxTemplateRequest"];
export type UpdateSandboxTemplateRequest = Schemas["updateSandboxTemplateRequest"];
export type PaginatedSandboxTemplates = Schemas["paginatedResponse-sandboxTemplateResponse"];

// Conversations
export type ConversationResponse = Schemas["conversationResponse"];
export type ConversationEventResponse = Schemas["conversationEventResponse"];
export type PaginatedConversations = Schemas["paginatedResponse-conversationResponse"];
export type PaginatedConversationEvents = Schemas["paginatedResponse-conversationEventResponse"];

// Sandboxes
export type SandboxResponse = Schemas["sandboxResponse"];
export type PaginatedSandboxes = Schemas["paginatedResponse-sandboxResponse"];
export type ExecRequest = Schemas["execRequest"];
export type ExecResponse = Schemas["execResponse"];
export type CommandResult = Schemas["commandResult"];

// Forge
export type StartForgeRequest = Schemas["startForgeRequest"];
export type ForgeEvent = Schemas["ForgeEvent"];
export type ForgeEvalResult = Schemas["ForgeEvalResult"];
export type ForgeIteration = Schemas["ForgeIteration"];

// Webhooks
export type WebhookSettingsResponse = Schemas["webhookSettingsResponse"];
export type WebhookSettingsCreateResponse = Schemas["webhookSettingsCreateResponse"];
