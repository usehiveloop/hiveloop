import * as openapi_fetch from 'openapi-fetch';
import openapi_fetch__default from 'openapi-fetch';

interface components {
    schemas: {
        "github_com_llmvault_llmvault_internal_mcp.TokenScope": {
            actions?: string[];
            connection_id?: string;
            resources?: {
                [key: string]: string[];
            };
        };
        "github_com_llmvault_llmvault_internal_model.JSON": {
            [key: string]: unknown;
        };
        "github_com_llmvault_llmvault_internal_nango.Credentials": {
            app_id?: string;
            app_link?: string;
            client_id?: string;
            client_logo_uri?: string;
            /** @description MCP_OAUTH2_GENERIC fields */
            client_name?: string;
            client_secret?: string;
            client_uri?: string;
            password?: string;
            private_key?: string;
            scopes?: string;
            type?: string;
            /** @description INSTALL_PLUGIN fields */
            username?: string;
            webhook_secret?: string;
        };
        "github_com_llmvault_llmvault_internal_registry.Cost": {
            input?: number;
            output?: number;
        };
        "github_com_llmvault_llmvault_internal_registry.Limit": {
            context?: number;
            output?: number;
        };
        "github_com_llmvault_llmvault_internal_registry.Modalities": {
            input?: string[];
            output?: string[];
        };
        "github_com_llmvault_llmvault_internal_resources.AvailableResource": {
            id?: string;
            name?: string;
            type?: string;
        };
        "github_com_llmvault_llmvault_internal_resources.DiscoveryResult": {
            resources?: components["schemas"]["github_com_llmvault_llmvault_internal_resources.AvailableResource"][];
        };
        "internal_handler.apiKeyResponse": {
            created_at?: string;
            expires_at?: string;
            id?: string;
            key_prefix?: string;
            last_used_at?: string;
            name?: string;
            revoked_at?: string;
            scopes?: string[];
        };
        "internal_handler.apiKeyStats": {
            active?: number;
            revoked?: number;
            total?: number;
        };
        "internal_handler.auditEntryResponse": {
            action?: string;
            created_at?: string;
            credential_id?: string;
            id?: number;
            identity_id?: string;
            ip_address?: string;
            latency_ms?: number;
            method?: string;
            path?: string;
            status?: number;
        };
        "internal_handler.connectSessionResponse": {
            allowed_origins?: string[];
            allowed_providers?: string[];
            created_at?: string;
            expires_at?: string;
            external_id?: string;
            id?: string;
            identity_id?: string;
            session_token?: string;
        };
        "internal_handler.connectSessionTokenResponse": {
            provider_config_key?: string;
            token?: string;
        };
        "internal_handler.connectSettingsRequest": {
            allowed_origins?: string[];
        };
        "internal_handler.connectSettingsResponse": {
            allowed_origins?: string[];
        };
        "internal_handler.connectionResponse": {
            auth_scheme?: string;
            base_url?: string;
            created_at?: string;
            id?: string;
            label?: string;
            provider_id?: string;
            provider_name?: string;
        };
        "internal_handler.createAPIKeyRequest": {
            /** @description Go duration, e.g. "720h" */
            expires_in?: string;
            name?: string;
            scopes?: string[];
        };
        "internal_handler.createAPIKeyResponse": {
            created_at?: string;
            expires_at?: string;
            id?: string;
            key?: string;
            key_prefix?: string;
            name?: string;
            scopes?: string[];
        };
        "internal_handler.createConnectSessionRequest": {
            allowed_origins?: string[];
            allowed_providers?: string[];
            external_id?: string;
            identity_id?: string;
            metadata?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            permissions?: string[];
            ttl?: string;
        };
        "internal_handler.createConnectionRequest": {
            api_key?: string;
            label?: string;
            provider_id?: string;
        };
        "internal_handler.createCredentialRequest": {
            api_key?: string;
            auth_scheme?: string;
            base_url?: string;
            /** @description auto-upserts identity */
            external_id?: string;
            identity_id?: string;
            label?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            refill_amount?: number;
            refill_interval?: string;
            remaining?: number;
        };
        "internal_handler.createIdentityRequest": {
            external_id?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            ratelimits?: components["schemas"]["internal_handler.identityRateLimitParams"][];
        };
        "internal_handler.createIntegrationConnectionRequest": {
            nango_connection_id?: string;
            resources?: {
                [key: string]: string[];
            };
        };
        "internal_handler.createIntegrationRequest": {
            credentials?: components["schemas"]["github_com_llmvault_llmvault_internal_nango.Credentials"];
            display_name?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            provider?: string;
        };
        "internal_handler.createOrgRequest": {
            name?: string;
        };
        "internal_handler.credentialResponse": {
            auth_scheme?: string;
            base_url?: string;
            created_at?: string;
            id?: string;
            identity_id?: string;
            label?: string;
            last_used_at?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            provider_id?: string;
            refill_amount?: number;
            refill_interval?: string;
            remaining?: number;
            request_count?: number;
            revoked_at?: string;
        };
        "internal_handler.credentialStats": {
            active?: number;
            revoked?: number;
            total?: number;
        };
        "internal_handler.dailyRequests": {
            count?: number;
            date?: string;
        };
        "internal_handler.errorResponse": {
            error?: string;
        };
        "internal_handler.identityRateLimitParams": {
            /** @description milliseconds */
            duration?: number;
            limit?: number;
            name?: string;
        };
        "internal_handler.identityResponse": {
            created_at?: string;
            external_id?: string;
            id?: string;
            last_used_at?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            ratelimits?: components["schemas"]["internal_handler.identityRateLimitParams"][];
            request_count?: number;
            updated_at?: string;
        };
        "internal_handler.identityStats": {
            total?: number;
        };
        "internal_handler.integConnCreateRequest": {
            identity_id?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            nango_connection_id?: string;
        };
        "internal_handler.integConnResponse": {
            created_at?: string;
            id?: string;
            identity_id?: string;
            integration_id?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            nango_connection_id?: string;
            revoked_at?: string;
            updated_at?: string;
        };
        "internal_handler.integrationProviderInfo": {
            auth_mode?: string;
            display_name?: string;
            name?: string;
        };
        "internal_handler.integrationResponse": {
            created_at?: string;
            display_name?: string;
            id?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            nango_config?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            provider?: string;
            updated_at?: string;
        };
        "internal_handler.mintTokenRequest": {
            credential_id?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            refill_amount?: number;
            refill_interval?: string;
            remaining?: number;
            scopes?: components["schemas"]["github_com_llmvault_llmvault_internal_mcp.TokenScope"][];
            /** @description e.g. "1h", "24h" */
            ttl?: string;
        };
        "internal_handler.mintTokenResponse": {
            expires_at?: string;
            jti?: string;
            token?: string;
        };
        "internal_handler.modelSummary": {
            cost?: components["schemas"]["github_com_llmvault_llmvault_internal_registry.Cost"];
            family?: string;
            id?: string;
            knowledge?: string;
            limit?: components["schemas"]["github_com_llmvault_llmvault_internal_registry.Limit"];
            modalities?: components["schemas"]["github_com_llmvault_llmvault_internal_registry.Modalities"];
            name?: string;
            open_weights?: boolean;
            reasoning?: boolean;
            release_date?: string;
            status?: string;
            structured_output?: boolean;
            tool_call?: boolean;
        };
        "internal_handler.orgResponse": {
            active?: boolean;
            created_at?: string;
            id?: string;
            logto_org_id?: string;
            name?: string;
            rate_limit?: number;
        };
        "internal_handler.paginatedResponse-internal_handler_apiKeyResponse": {
            data?: components["schemas"]["internal_handler.apiKeyResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "internal_handler.paginatedResponse-internal_handler_auditEntryResponse": {
            data?: components["schemas"]["internal_handler.auditEntryResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "internal_handler.paginatedResponse-internal_handler_connectionResponse": {
            data?: components["schemas"]["internal_handler.connectionResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "internal_handler.paginatedResponse-internal_handler_credentialResponse": {
            data?: components["schemas"]["internal_handler.credentialResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "internal_handler.paginatedResponse-internal_handler_identityResponse": {
            data?: components["schemas"]["internal_handler.identityResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "internal_handler.paginatedResponse-internal_handler_integConnResponse": {
            data?: components["schemas"]["internal_handler.integConnResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "internal_handler.paginatedResponse-internal_handler_integrationResponse": {
            data?: components["schemas"]["internal_handler.integrationResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "internal_handler.patchIntegrationConnectionRequest": {
            resources?: {
                [key: string]: string[];
            };
        };
        "internal_handler.providerDetail": {
            api?: string;
            doc?: string;
            id?: string;
            models?: components["schemas"]["internal_handler.modelSummary"][];
            name?: string;
        };
        "internal_handler.providerSummary": {
            api?: string;
            doc?: string;
            id?: string;
            model_count?: number;
            name?: string;
        };
        "internal_handler.requestStats": {
            last_30d?: number;
            last_7d?: number;
            today?: number;
            total?: number;
            yesterday?: number;
        };
        "internal_handler.sessionInfoResponse": {
            activated_at?: string;
            allowed_providers?: string[];
            expires_at?: string;
            external_id?: string;
            id?: string;
            identity_id?: string;
            permissions?: string[];
        };
        "internal_handler.tokenStats": {
            active?: number;
            expired?: number;
            revoked?: number;
            total?: number;
        };
        "internal_handler.topCredential": {
            id?: string;
            label?: string;
            provider_id?: string;
            request_count?: number;
        };
        "internal_handler.updateIdentityRequest": {
            external_id?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            ratelimits?: components["schemas"]["internal_handler.identityRateLimitParams"][];
        };
        "internal_handler.updateIntegrationRequest": {
            credentials?: components["schemas"]["github_com_llmvault_llmvault_internal_nango.Credentials"];
            display_name?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
        };
        "internal_handler.usageResponse": {
            api_keys?: components["schemas"]["internal_handler.apiKeyStats"];
            credentials?: components["schemas"]["internal_handler.credentialStats"];
            daily_requests?: components["schemas"]["internal_handler.dailyRequests"][];
            identities?: components["schemas"]["internal_handler.identityStats"];
            requests?: components["schemas"]["internal_handler.requestStats"];
            tokens?: components["schemas"]["internal_handler.tokenStats"];
            top_credentials?: components["schemas"]["internal_handler.topCredential"][];
        };
        "internal_handler.widgetIntegrationResponse": {
            auth_mode?: string;
            connection_id?: string;
            display_name?: string;
            id?: string;
            nango_connection_id?: string;
            provider?: string;
            resources?: components["schemas"]["internal_handler.widgetResourceResponse"][];
            selected_resources?: {
                [key: string]: string[];
            };
            unique_key?: string;
        };
        "internal_handler.widgetResourceResponse": {
            description?: string;
            display_name?: string;
            icon?: string;
            type?: string;
        };
    };
    responses: never;
    parameters: never;
    requestBodies: never;
    headers: never;
    pathItems: never;
}

interface LLMVaultConfig {
    apiKey: string;
    baseUrl?: string;
}
type Schemas = components["schemas"];
type ApiKeyResponse = Schemas["internal_handler.apiKeyResponse"];
type CreateAPIKeyRequest = Schemas["internal_handler.createAPIKeyRequest"];
type CreateAPIKeyResponse = Schemas["internal_handler.createAPIKeyResponse"];
type CredentialResponse = Schemas["internal_handler.credentialResponse"];
type CreateCredentialRequest = Schemas["internal_handler.createCredentialRequest"];
type MintTokenRequest = Schemas["internal_handler.mintTokenRequest"];
type MintTokenResponse = Schemas["internal_handler.mintTokenResponse"];
type TokenScope = Schemas["github_com_llmvault_llmvault_internal_mcp.TokenScope"];
type IdentityResponse = Schemas["internal_handler.identityResponse"];
type CreateIdentityRequest = Schemas["internal_handler.createIdentityRequest"];
type UpdateIdentityRequest = Schemas["internal_handler.updateIdentityRequest"];
type IdentityRateLimitParams = Schemas["internal_handler.identityRateLimitParams"];
type ConnectSessionResponse = Schemas["internal_handler.connectSessionResponse"];
type CreateConnectSessionRequest = Schemas["internal_handler.createConnectSessionRequest"];
type ConnectSettingsRequest = Schemas["internal_handler.connectSettingsRequest"];
type ConnectSettingsResponse = Schemas["internal_handler.connectSettingsResponse"];
type IntegrationResponse = Schemas["internal_handler.integrationResponse"];
type CreateIntegrationRequest = Schemas["internal_handler.createIntegrationRequest"];
type UpdateIntegrationRequest = Schemas["internal_handler.updateIntegrationRequest"];
type NangoCredentials = Schemas["github_com_llmvault_llmvault_internal_nango.Credentials"];
type IntegConnResponse = Schemas["internal_handler.integConnResponse"];
type IntegConnCreateRequest = Schemas["internal_handler.integConnCreateRequest"];
type UsageResponse = Schemas["internal_handler.usageResponse"];
type AuditEntryResponse = Schemas["internal_handler.auditEntryResponse"];
type OrgResponse = Schemas["internal_handler.orgResponse"];
type ProviderSummary = Schemas["internal_handler.providerSummary"];
type ProviderDetail = Schemas["internal_handler.providerDetail"];
type ModelSummary = Schemas["internal_handler.modelSummary"];
type PaginatedApiKeys = Schemas["internal_handler.paginatedResponse-internal_handler_apiKeyResponse"];
type PaginatedCredentials = Schemas["internal_handler.paginatedResponse-internal_handler_credentialResponse"];
type PaginatedIdentities = Schemas["internal_handler.paginatedResponse-internal_handler_identityResponse"];
type PaginatedAuditEntries = Schemas["internal_handler.paginatedResponse-internal_handler_auditEntryResponse"];
type PaginatedIntegrations = Schemas["internal_handler.paginatedResponse-internal_handler_integrationResponse"];
type PaginatedIntegConns = Schemas["internal_handler.paginatedResponse-internal_handler_integConnResponse"];
type ErrorResponse = Schemas["internal_handler.errorResponse"];
type JSON = Schemas["github_com_llmvault_llmvault_internal_model.JSON"];

type ApiClient = ReturnType<typeof openapi_fetch__default<paths>>;
declare class BaseResource {
    protected client: ApiClient;
    constructor(client: ApiClient);
}

declare class ApiKeysResource extends BaseResource {
    create(body: CreateAPIKeyRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["internal_handler.createAPIKeyRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.createAPIKeyResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        body: {
            expires_in?: string;
            name?: string;
            scopes?: string[];
        };
    }, `${string}/${string}`>>;
    list(query?: {
        limit?: number;
        cursor?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.paginatedResponse-internal_handler_apiKeyResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            query: {
                limit?: number;
                cursor?: string;
            } | undefined;
        };
    }, `${string}/${string}`>>;
    delete(id: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        [key: string]: string;
                    };
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
    }, `${string}/${string}`>>;
}

declare class CredentialsResource extends BaseResource {
    create(body: CreateCredentialRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["internal_handler.createCredentialRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.credentialResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        body: {
            api_key?: string;
            auth_scheme?: string;
            base_url?: string;
            external_id?: string;
            identity_id?: string;
            label?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            refill_amount?: number;
            refill_interval?: string;
            remaining?: number;
        };
    }, `${string}/${string}`>>;
    list(query?: {
        limit?: number;
        cursor?: string;
        identity_id?: string;
        external_id?: string;
        meta?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                identity_id?: string;
                external_id?: string;
                meta?: string;
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.paginatedResponse-internal_handler_credentialResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            query: {
                limit?: number;
                cursor?: string;
                identity_id?: string;
                external_id?: string;
                meta?: string;
            } | undefined;
        };
    }, `${string}/${string}`>>;
    delete(id: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.credentialResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
    }, `${string}/${string}`>>;
}

declare class TokensResource extends BaseResource {
    create(body: MintTokenRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["internal_handler.mintTokenRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.mintTokenResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        body: {
            credential_id?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            refill_amount?: number;
            refill_interval?: string;
            remaining?: number;
            scopes?: components["schemas"]["github_com_llmvault_llmvault_internal_mcp.TokenScope"][];
            ttl?: string;
        };
    }, `${string}/${string}`>>;
    delete(jti: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                jti: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        [key: string]: string;
                    };
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                jti: string;
            };
        };
    }, `${string}/${string}`>>;
}

declare class IdentitiesResource extends BaseResource {
    create(body: CreateIdentityRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["internal_handler.createIdentityRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.identityResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        body: {
            external_id?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            ratelimits?: components["schemas"]["internal_handler.identityRateLimitParams"][];
        };
    }, `${string}/${string}`>>;
    list(query?: {
        limit?: number;
        cursor?: string;
        external_id?: string;
        meta?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                external_id?: string;
                meta?: string;
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.paginatedResponse-internal_handler_identityResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            query: {
                limit?: number;
                cursor?: string;
                external_id?: string;
                meta?: string;
            } | undefined;
        };
    }, `${string}/${string}`>>;
    get(id: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.identityResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
    }, `${string}/${string}`>>;
    update(id: string, body: UpdateIdentityRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["internal_handler.updateIdentityRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.identityResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
        body: {
            external_id?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            ratelimits?: components["schemas"]["internal_handler.identityRateLimitParams"][];
        };
    }, `${string}/${string}`>>;
    delete(id: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        [key: string]: string;
                    };
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
    }, `${string}/${string}`>>;
}

declare class ConnectSessionsResource extends BaseResource {
    create(body: CreateConnectSessionRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["internal_handler.createConnectSessionRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.connectSessionResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        body: {
            allowed_origins?: string[];
            allowed_providers?: string[];
            external_id?: string;
            identity_id?: string;
            metadata?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            permissions?: string[];
            ttl?: string;
        };
    }, `${string}/${string}`>>;
}
declare class ConnectSettingsResource extends BaseResource {
    get(): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.connectSettingsResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, openapi_fetch.FetchOptions<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.connectSettingsResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }> | undefined, `${string}/${string}`>>;
    update(body: ConnectSettingsRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["internal_handler.connectSettingsRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.connectSettingsResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        body: {
            allowed_origins?: string[];
        };
    }, `${string}/${string}`>>;
}
declare class ConnectResource extends BaseResource {
    readonly sessions: ConnectSessionsResource;
    readonly settings: ConnectSettingsResource;
    constructor(client: ConstructorParameters<typeof BaseResource>[0]);
}

declare class IntegrationsResource extends BaseResource {
    create(body: CreateIntegrationRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["internal_handler.createIntegrationRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.integrationResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            502: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        body: {
            credentials?: components["schemas"]["github_com_llmvault_llmvault_internal_nango.Credentials"];
            display_name?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            provider?: string;
        };
    }, `${string}/${string}`>>;
    list(query?: {
        limit?: number;
        cursor?: string;
        provider?: string;
        meta?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                limit?: number;
                cursor?: string;
                provider?: string;
                meta?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.paginatedResponse-internal_handler_integrationResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            query: {
                limit?: number;
                cursor?: string;
                provider?: string;
                meta?: string;
            } | undefined;
        };
    }, `${string}/${string}`>>;
    get(id: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.integrationResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
    }, `${string}/${string}`>>;
    update(id: string, body: UpdateIntegrationRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["internal_handler.updateIntegrationRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.integrationResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            502: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
        body: {
            credentials?: components["schemas"]["github_com_llmvault_llmvault_internal_nango.Credentials"];
            display_name?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
        };
    }, `${string}/${string}`>>;
    delete(id: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        [key: string]: string;
                    };
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            502: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
    }, `${string}/${string}`>>;
    listProviders(): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.integrationProviderInfo"][];
                };
            };
        };
    }, openapi_fetch.FetchOptions<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.integrationProviderInfo"][];
                };
            };
        };
    }> | undefined, `${string}/${string}`>>;
}

declare class ConnectionsResource extends BaseResource {
    create(integrationId: string, body: IntegConnCreateRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["internal_handler.integConnCreateRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.integConnResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
        body: {
            identity_id?: string;
            meta?: components["schemas"]["github_com_llmvault_llmvault_internal_model.JSON"];
            nango_connection_id?: string;
        };
    }, `${string}/${string}`>>;
    list(integrationId: string, query?: {
        limit?: number;
        cursor?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.paginatedResponse-internal_handler_integConnResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
            query: {
                limit?: number;
                cursor?: string;
            } | undefined;
        };
    }, `${string}/${string}`>>;
    get(id: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.integConnResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
    }, `${string}/${string}`>>;
    delete(id: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        [key: string]: string;
                    };
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
    }, `${string}/${string}`>>;
}

declare class UsageResource extends BaseResource {
    get(): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.usageResponse"];
                };
            };
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, openapi_fetch.FetchOptions<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.usageResponse"];
                };
            };
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }> | undefined, `${string}/${string}`>>;
}

declare class AuditResource extends BaseResource {
    list(query?: {
        limit?: number;
        cursor?: string;
        action?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                limit?: number;
                cursor?: string;
                action?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.paginatedResponse-internal_handler_auditEntryResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            query: {
                limit?: number;
                cursor?: string;
                action?: string;
            } | undefined;
        };
    }, `${string}/${string}`>>;
}

declare class OrgResource extends BaseResource {
    getCurrent(): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.orgResponse"];
                };
            };
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, openapi_fetch.FetchOptions<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.orgResponse"];
                };
            };
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }> | undefined, `${string}/${string}`>>;
}

declare class ProvidersResource extends BaseResource {
    list(): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.providerSummary"][];
                };
            };
        };
    }, openapi_fetch.FetchOptions<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.providerSummary"][];
                };
            };
        };
    }> | undefined, `${string}/${string}`>>;
    get(id: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.providerDetail"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
    }, `${string}/${string}`>>;
    listModels(id: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.modelSummary"][];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["internal_handler.errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
    }, `${string}/${string}`>>;
}

declare class LLMVault {
    readonly apiKeys: ApiKeysResource;
    readonly credentials: CredentialsResource;
    readonly tokens: TokensResource;
    readonly identities: IdentitiesResource;
    readonly connect: ConnectResource;
    readonly integrations: IntegrationsResource;
    readonly connections: ConnectionsResource;
    readonly usage: UsageResource;
    readonly audit: AuditResource;
    readonly org: OrgResource;
    readonly providers: ProvidersResource;
    constructor(config: LLMVaultConfig);
}

export { type ApiKeyResponse, type AuditEntryResponse, type ConnectSessionResponse, type ConnectSettingsRequest, type ConnectSettingsResponse, type CreateAPIKeyRequest, type CreateAPIKeyResponse, type CreateConnectSessionRequest, type CreateCredentialRequest, type CreateIdentityRequest, type CreateIntegrationRequest, type CredentialResponse, type ErrorResponse, type IdentityRateLimitParams, type IdentityResponse, type IntegConnCreateRequest, type IntegConnResponse, type IntegrationResponse, type JSON, LLMVault, type LLMVaultConfig, type MintTokenRequest, type MintTokenResponse, type ModelSummary, type NangoCredentials, type OrgResponse, type PaginatedApiKeys, type PaginatedAuditEntries, type PaginatedCredentials, type PaginatedIdentities, type PaginatedIntegConns, type PaginatedIntegrations, type ProviderDetail, type ProviderSummary, type TokenScope, type UpdateIdentityRequest, type UpdateIntegrationRequest, type UsageResponse };
