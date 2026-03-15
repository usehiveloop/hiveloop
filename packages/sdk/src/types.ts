import type { components } from "./generated/schema.js";

export interface LLMVaultConfig {
  apiKey: string;
  baseUrl?: string;
}

// Re-export schema types with friendly aliases
type Schemas = components["schemas"];

export type ApiKeyResponse = Schemas["internal_handler.apiKeyResponse"];
export type CreateAPIKeyRequest = Schemas["internal_handler.createAPIKeyRequest"];
export type CreateAPIKeyResponse = Schemas["internal_handler.createAPIKeyResponse"];

export type CredentialResponse = Schemas["internal_handler.credentialResponse"];
export type CreateCredentialRequest = Schemas["internal_handler.createCredentialRequest"];

export type MintTokenRequest = Schemas["internal_handler.mintTokenRequest"];
export type MintTokenResponse = Schemas["internal_handler.mintTokenResponse"];
export type TokenScope = Schemas["github_com_llmvault_llmvault_internal_mcp.TokenScope"];

export type IdentityResponse = Schemas["internal_handler.identityResponse"];
export type CreateIdentityRequest = Schemas["internal_handler.createIdentityRequest"];
export type UpdateIdentityRequest = Schemas["internal_handler.updateIdentityRequest"];
export type IdentityRateLimitParams = Schemas["internal_handler.identityRateLimitParams"];

export type ConnectSessionResponse = Schemas["internal_handler.connectSessionResponse"];
export type CreateConnectSessionRequest = Schemas["internal_handler.createConnectSessionRequest"];
export type ConnectSettingsRequest = Schemas["internal_handler.connectSettingsRequest"];
export type ConnectSettingsResponse = Schemas["internal_handler.connectSettingsResponse"];

export type IntegrationResponse = Schemas["internal_handler.integrationResponse"];
export type CreateIntegrationRequest = Schemas["internal_handler.createIntegrationRequest"];
export type UpdateIntegrationRequest = Schemas["internal_handler.updateIntegrationRequest"];
export type NangoCredentials = Schemas["github_com_llmvault_llmvault_internal_nango.Credentials"];

export type IntegConnResponse = Schemas["internal_handler.integConnResponse"];
export type IntegConnCreateRequest = Schemas["internal_handler.integConnCreateRequest"];

export type UsageResponse = Schemas["internal_handler.usageResponse"];
export type AuditEntryResponse = Schemas["internal_handler.auditEntryResponse"];

export type OrgResponse = Schemas["internal_handler.orgResponse"];

export type ProviderSummary = Schemas["internal_handler.providerSummary"];
export type ProviderDetail = Schemas["internal_handler.providerDetail"];
export type ModelSummary = Schemas["internal_handler.modelSummary"];

export type PaginatedApiKeys = Schemas["internal_handler.paginatedResponse-internal_handler_apiKeyResponse"];
export type PaginatedCredentials = Schemas["internal_handler.paginatedResponse-internal_handler_credentialResponse"];
export type PaginatedIdentities = Schemas["internal_handler.paginatedResponse-internal_handler_identityResponse"];
export type PaginatedAuditEntries = Schemas["internal_handler.paginatedResponse-internal_handler_auditEntryResponse"];
export type PaginatedIntegrations = Schemas["internal_handler.paginatedResponse-internal_handler_integrationResponse"];
export type PaginatedIntegConns = Schemas["internal_handler.paginatedResponse-internal_handler_integConnResponse"];

export type ErrorResponse = Schemas["internal_handler.errorResponse"];
export type JSON = Schemas["github_com_llmvault_llmvault_internal_model.JSON"];
