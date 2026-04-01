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
        JSON: {
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
        Cost: {
            input?: number;
            output?: number;
        };
        Limit: {
            context?: number;
            output?: number;
        };
        Modalities: {
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
        actionSummary: {
            access?: string;
            description?: string;
            display_name?: string;
            key?: string;
            parameters?: number[];
            resource_type?: string;
        };
        agentResponse: {
            agent_config?: components["schemas"]["JSON"];
            created_at?: string;
            credential_id?: string;
            description?: string;
            id?: string;
            identity_id?: string;
            integrations?: components["schemas"]["JSON"];
            mcp_servers?: components["schemas"]["JSON"];
            model?: string;
            name?: string;
            permissions?: components["schemas"]["JSON"];
            provider_id?: string;
            sandbox_template_id?: string;
            sandbox_type?: string;
            skills?: components["schemas"]["JSON"];
            status?: string;
            subagents?: components["schemas"]["JSON"];
            system_prompt?: string;
            tools?: components["schemas"]["JSON"];
            updated_at?: string;
        };
        apiKeyResponse: {
            created_at?: string;
            expires_at?: string;
            id?: string;
            key_prefix?: string;
            last_used_at?: string;
            name?: string;
            revoked_at?: string;
            scopes?: string[];
        };
        apiKeyStats: {
            active?: number;
            revoked?: number;
            total?: number;
        };
        auditEntryResponse: {
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
        authResponse: {
            access_token?: string;
            /** @description seconds */
            expires_in?: number;
            orgs?: components["schemas"]["orgMemberDTO"][];
            refresh_token?: string;
            user?: components["schemas"]["userResponse"];
        };
        availableScopeAction: {
            description?: string;
            display_name?: string;
            key?: string;
            resource_type?: string;
        };
        availableScopeConnection: {
            actions?: components["schemas"]["availableScopeAction"][];
            connection_id?: string;
            display_name?: string;
            integration_id?: string;
            provider?: string;
            resources?: {
                [key: string]: components["schemas"]["availableScopeResource"];
            };
        };
        availableScopeResource: {
            display_name?: string;
            selected?: components["schemas"]["availableScopeResourceItem"][];
        };
        availableScopeResourceItem: {
            id?: string;
            name?: string;
        };
        changePasswordRequest: {
            current_password?: string;
            new_password?: string;
        };
        commandResult: {
            command?: string;
            error?: string;
            exit_code?: number;
            output?: string;
        };
        confirmEmailRequest: {
            token?: string;
        };
        connectSessionListItem: {
            activated_at?: string;
            allowed_integrations?: string[];
            allowed_origins?: string[];
            created_at?: string;
            expires_at?: string;
            external_id?: string;
            id?: string;
            identity_id?: string;
            metadata?: components["schemas"]["JSON"];
            permissions?: string[];
            session_token?: string;
            status?: string;
        };
        connectSessionResponse: {
            allowed_integrations?: string[];
            allowed_origins?: string[];
            created_at?: string;
            expires_at?: string;
            external_id?: string;
            id?: string;
            identity_id?: string;
            session_token?: string;
        };
        connectSessionTokenResponse: {
            provider_config_key?: string;
            token?: string;
        };
        connectSettingsRequest: {
            allowed_origins?: string[];
        };
        connectSettingsResponse: {
            allowed_origins?: string[];
        };
        connectionResponse: {
            auth_scheme?: string;
            base_url?: string;
            created_at?: string;
            id?: string;
            label?: string;
            provider_id?: string;
            provider_name?: string;
        };
        conversationEventResponse: {
            created_at?: string;
            event_type?: string;
            id?: string;
            payload?: components["schemas"]["JSON"];
        };
        conversationResponse: {
            agent_id?: string;
            created_at?: string;
            id?: string;
            status?: string;
            stream_url?: string;
        };
        createAPIKeyRequest: {
            expires_in?: string;
            name?: string;
            scopes?: string[];
        };
        createAPIKeyResponse: {
            created_at?: string;
            expires_at?: string;
            id?: string;
            key?: string;
            key_prefix?: string;
            name?: string;
            scopes?: string[];
        };
        createAgentRequest: {
            agent_config?: components["schemas"]["JSON"];
            credential_id?: string;
            description?: string;
            identity_id?: string;
            integrations?: components["schemas"]["JSON"];
            mcp_servers?: components["schemas"]["JSON"];
            model?: string;
            name?: string;
            permissions?: components["schemas"]["JSON"];
            sandbox_template_id?: string;
            sandbox_type?: string;
            skills?: components["schemas"]["JSON"];
            subagents?: components["schemas"]["JSON"];
            system_prompt?: string;
            tools?: components["schemas"]["JSON"];
        };
        createConnectSessionRequest: {
            allowed_integrations?: string[];
            allowed_origins?: string[];
            external_id?: string;
            identity_id?: string;
            metadata?: components["schemas"]["JSON"];
            permissions?: string[];
            ttl?: string;
        };
        createConnectionRequest: {
            api_key?: string;
            label?: string;
            provider_id?: string;
        };
        createCredentialRequest: {
            api_key?: string;
            auth_scheme?: string;
            base_url?: string;
            external_id?: string;
            identity_id?: string;
            label?: string;
            meta?: components["schemas"]["JSON"];
            provider_id?: string;
            refill_amount?: number;
            refill_interval?: string;
            remaining?: number;
        };
        createIdentityRequest: {
            external_id?: string;
            meta?: components["schemas"]["JSON"];
            ratelimits?: components["schemas"]["identityRateLimitParams"][];
        };
        createIntegrationConnectionRequest: {
            nango_connection_id?: string;
            resources?: {
                [key: string]: string[];
            };
        };
        createIntegrationRequest: {
            credentials?: components["schemas"]["github_com_llmvault_llmvault_internal_nango.Credentials"];
            display_name?: string;
            meta?: components["schemas"]["JSON"];
            provider?: string;
        };
        createOrgRequest: {
            name?: string;
        };
        createSandboxTemplateRequest: {
            build_commands?: string;
            config?: components["schemas"]["JSON"];
            name?: string;
        };
        credentialResponse: {
            auth_scheme?: string;
            base_url?: string;
            created_at?: string;
            id?: string;
            identity_id?: string;
            label?: string;
            last_used_at?: string;
            meta?: components["schemas"]["JSON"];
            provider_id?: string;
            refill_amount?: number;
            refill_interval?: string;
            remaining?: number;
            request_count?: number;
            revoked_at?: string;
        };
        credentialStats: {
            active?: number;
            revoked?: number;
            total?: number;
        };
        dailyRequests: {
            count?: number;
            date?: string;
        };
        errorRate: {
            date?: string;
            error_count?: number;
            total?: number;
        };
        errorResponse: {
            error?: string;
        };
        execRequest: {
            commands?: string[];
        };
        execResponse: {
            results?: components["schemas"]["commandResult"][];
            success?: boolean;
        };
        forgotPasswordRequest: {
            email?: string;
        };
        identityRateLimitParams: {
            /** @description milliseconds */
            duration?: number;
            limit?: number;
            name?: string;
        };
        identityResponse: {
            created_at?: string;
            external_id?: string;
            id?: string;
            last_used_at?: string;
            meta?: components["schemas"]["JSON"];
            ratelimits?: components["schemas"]["identityRateLimitParams"][];
            request_count?: number;
            updated_at?: string;
        };
        identityStats: {
            total?: number;
        };
        integConnCreateRequest: {
            identity_id?: string;
            meta?: components["schemas"]["JSON"];
            nango_connection_id?: string;
        };
        integConnResponse: {
            created_at?: string;
            id?: string;
            identity_id?: string;
            integration_id?: string;
            meta?: components["schemas"]["JSON"];
            nango_connection_id?: string;
            provider_config?: components["schemas"]["JSON"];
            revoked_at?: string;
            updated_at?: string;
        };
        integrationDetail: {
            actions?: components["schemas"]["actionSummary"][];
            display_name?: string;
            id?: string;
            resources?: {
                [key: string]: components["schemas"]["resource"];
            };
        };
        integrationProviderInfo: {
            auth_mode?: string;
            display_name?: string;
            name?: string;
            webhook_user_defined_secret?: boolean;
        };
        integrationResponse: {
            created_at?: string;
            display_name?: string;
            id?: string;
            meta?: components["schemas"]["JSON"];
            nango_config?: components["schemas"]["JSON"];
            provider?: string;
            unique_key?: string;
            updated_at?: string;
        };
        integrationSummary: {
            action_count?: number;
            display_name?: string;
            has_resources?: boolean;
            id?: string;
            read_count?: number;
            write_count?: number;
        };
        latencyStats: {
            avg_ttfb_ms?: number;
            date?: string;
            p95_ttfb_ms?: number;
        };
        loginRequest: {
            email?: string;
            /** @description optional: scope token to a specific org */
            org_id?: string;
            password?: string;
        };
        logoutRequest: {
            refresh_token?: string;
        };
        meResponse: {
            orgs?: components["schemas"]["orgMemberDTO"][];
            user?: components["schemas"]["userResponse"];
        };
        mintTokenRequest: {
            credential_id?: string;
            meta?: components["schemas"]["JSON"];
            refill_amount?: number;
            refill_interval?: string;
            remaining?: number;
            scopes?: components["schemas"]["github_com_llmvault_llmvault_internal_mcp.TokenScope"][];
            /** @description e.g. "1h", "24h" */
            ttl?: string;
        };
        mintTokenResponse: {
            expires_at?: string;
            jti?: string;
            mcp_endpoint?: string;
            token?: string;
        };
        modelSummary: {
            cost?: components["schemas"]["Cost"];
            family?: string;
            id?: string;
            knowledge?: string;
            limit?: components["schemas"]["Limit"];
            modalities?: components["schemas"]["Modalities"];
            name?: string;
            open_weights?: boolean;
            reasoning?: boolean;
            release_date?: string;
            status?: string;
            structured_output?: boolean;
            tool_call?: boolean;
        };
        orgMemberDTO: {
            id?: string;
            name?: string;
            role?: string;
        };
        orgResponse: {
            active?: boolean;
            created_at?: string;
            id?: string;
            name?: string;
            rate_limit?: number;
        };
        "paginatedResponse-agentResponse": {
            data?: components["schemas"]["agentResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-apiKeyResponse": {
            data?: components["schemas"]["apiKeyResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-auditEntryResponse": {
            data?: components["schemas"]["auditEntryResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-connectSessionListItem": {
            data?: components["schemas"]["connectSessionListItem"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-connectionResponse": {
            data?: components["schemas"]["connectionResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-conversationEventResponse": {
            data?: components["schemas"]["conversationEventResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-conversationResponse": {
            data?: components["schemas"]["conversationResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-credentialResponse": {
            data?: components["schemas"]["credentialResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-identityResponse": {
            data?: components["schemas"]["identityResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-integConnResponse": {
            data?: components["schemas"]["integConnResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-integrationResponse": {
            data?: components["schemas"]["integrationResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-sandboxResponse": {
            data?: components["schemas"]["sandboxResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-sandboxTemplateResponse": {
            data?: components["schemas"]["sandboxTemplateResponse"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        "paginatedResponse-tokenListItem": {
            data?: components["schemas"]["tokenListItem"][];
            has_more?: boolean;
            next_cursor?: string;
        };
        patchIntegrationConnectionRequest: {
            resources?: {
                [key: string]: string[];
            };
        };
        providerDetail: {
            api?: string;
            doc?: string;
            id?: string;
            models?: components["schemas"]["modelSummary"][];
            name?: string;
        };
        providerSummary: {
            api?: string;
            doc?: string;
            id?: string;
            model_count?: number;
            name?: string;
        };
        refreshRequest: {
            /** @description optional: switch org */
            org_id?: string;
            refresh_token?: string;
        };
        registerRequest: {
            email?: string;
            name?: string;
            password?: string;
        };
        reportRow: {
            avg_ttfb_ms?: number;
            cached_tokens?: number;
            credential_id?: string;
            error_count?: number;
            identity_id?: string;
            input_tokens?: number;
            model?: string;
            output_tokens?: number;
            p50_ttfb_ms?: number;
            p95_ttfb_ms?: number;
            period?: string;
            provider_id?: string;
            reasoning_tokens?: number;
            request_count?: number;
            total_cost?: number;
            user_id?: string;
        };
        requestStats: {
            last_30d?: number;
            last_7d?: number;
            today?: number;
            total?: number;
            yesterday?: number;
        };
        resendConfirmationRequest: {
            email?: string;
        };
        resetPasswordRequest: {
            new_password?: string;
            token?: string;
        };
        resource: {
            description?: string;
            display_name?: string;
            icon?: string;
            id_field?: string;
            name_field?: string;
        };
        sandboxResponse: {
            agent_id?: string;
            created_at?: string;
            error_message?: string;
            external_id?: string;
            id?: string;
            identity_id?: string;
            last_active_at?: string;
            sandbox_type?: string;
            status?: string;
        };
        sandboxTemplateResponse: {
            build_commands?: string;
            build_error?: string;
            build_status?: string;
            config?: components["schemas"]["JSON"];
            created_at?: string;
            external_id?: string;
            id?: string;
            name?: string;
            updated_at?: string;
        };
        sessionInfoResponse: {
            activated_at?: string;
            allowed_integrations?: string[];
            expires_at?: string;
            external_id?: string;
            id?: string;
            identity_id?: string;
            permissions?: string[];
        };
        spendOverTime: {
            date?: string;
            total_cost?: number;
        };
        statusResponse: {
            message?: string;
            status?: string;
        };
        tokenListItem: {
            created_at?: string;
            credential_id?: string;
            expires_at?: string;
            id?: string;
            jti?: string;
            meta?: components["schemas"]["JSON"];
            refill_amount?: number;
            refill_interval?: string;
            remaining?: number;
            revoked_at?: string;
            scopes?: components["schemas"]["JSON"];
        };
        tokenStats: {
            active?: number;
            expired?: number;
            revoked?: number;
            total?: number;
        };
        tokenVolumes: {
            cached_tokens?: number;
            date?: string;
            input_tokens?: number;
            output_tokens?: number;
        };
        topCredential: {
            id?: string;
            label?: string;
            provider_id?: string;
            request_count?: number;
        };
        topModel: {
            model?: string;
            provider_id?: string;
            request_count?: number;
            total_cost?: number;
        };
        topUser: {
            request_count?: number;
            total_cost?: number;
            user_id?: string;
        };
        updateAgentRequest: {
            agent_config?: components["schemas"]["JSON"];
            credential_id?: string;
            description?: string;
            integrations?: components["schemas"]["JSON"];
            mcp_servers?: components["schemas"]["JSON"];
            model?: string;
            name?: string;
            permissions?: components["schemas"]["JSON"];
            sandbox_template_id?: string;
            sandbox_type?: string;
            skills?: components["schemas"]["JSON"];
            subagents?: components["schemas"]["JSON"];
            system_prompt?: string;
            tools?: components["schemas"]["JSON"];
        };
        updateIdentityRequest: {
            external_id?: string;
            meta?: components["schemas"]["JSON"];
            ratelimits?: components["schemas"]["identityRateLimitParams"][];
        };
        updateIntegrationRequest: {
            credentials?: components["schemas"]["github_com_llmvault_llmvault_internal_nango.Credentials"];
            display_name?: string;
            meta?: components["schemas"]["JSON"];
        };
        updateSandboxTemplateRequest: {
            build_commands?: string;
            config?: components["schemas"]["JSON"];
            name?: string;
        };
        usageResponse: {
            api_keys?: components["schemas"]["apiKeyStats"];
            credentials?: components["schemas"]["credentialStats"];
            daily_requests?: components["schemas"]["dailyRequests"][];
            error_rates?: components["schemas"]["errorRate"][];
            identities?: components["schemas"]["identityStats"];
            latency?: components["schemas"]["latencyStats"][];
            requests?: components["schemas"]["requestStats"];
            /** @description Generation-based analytics */
            spend_over_time?: components["schemas"]["spendOverTime"][];
            token_volumes?: components["schemas"]["tokenVolumes"][];
            tokens?: components["schemas"]["tokenStats"];
            top_credentials?: components["schemas"]["topCredential"][];
            top_models?: components["schemas"]["topModel"][];
            top_users?: components["schemas"]["topUser"][];
        };
        userResponse: {
            email?: string;
            email_confirmed?: boolean;
            id?: string;
            name?: string;
        };
        widgetIntegrationResponse: {
            auth_mode?: string;
            connection_id?: string;
            display_name?: string;
            id?: string;
            nango_connection_id?: string;
            provider?: string;
            resources?: components["schemas"]["widgetResourceResponse"][];
            selected_resources?: {
                [key: string]: string[];
            };
            unique_key?: string;
        };
        widgetResourceResponse: {
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
type ApiKeyResponse = Schemas["apiKeyResponse"];
type CreateAPIKeyRequest = Schemas["createAPIKeyRequest"];
type CreateAPIKeyResponse = Schemas["createAPIKeyResponse"];
type CredentialResponse = Schemas["credentialResponse"];
type CreateCredentialRequest = Schemas["createCredentialRequest"];
type MintTokenRequest = Schemas["mintTokenRequest"];
type MintTokenResponse = Schemas["mintTokenResponse"];
type TokenListItem = Schemas["tokenListItem"];
type PaginatedTokens = Schemas["paginatedResponse-tokenListItem"];
type TokenScope = Schemas["github_com_llmvault_llmvault_internal_mcp.TokenScope"];
interface AvailableScopeAction {
    key: string;
    display_name: string;
    description: string;
    resource_type?: string;
}
interface AvailableScopeResourceItem {
    id: string;
    name: string;
}
interface AvailableScopeResource {
    display_name: string;
    selected: AvailableScopeResourceItem[];
}
interface AvailableScopeConnection {
    connection_id: string;
    integration_id: string;
    provider: string;
    display_name: string;
    actions: AvailableScopeAction[];
    resources?: Record<string, AvailableScopeResource>;
}
type IdentityResponse = Schemas["identityResponse"];
type CreateIdentityRequest = Schemas["createIdentityRequest"];
type UpdateIdentityRequest = Schemas["updateIdentityRequest"];
type IdentityRateLimitParams = Schemas["identityRateLimitParams"];
type ConnectSessionResponse = Schemas["connectSessionResponse"];
type CreateConnectSessionRequest = Schemas["createConnectSessionRequest"];
type ConnectSettingsRequest = Schemas["connectSettingsRequest"];
type ConnectSettingsResponse = Schemas["connectSettingsResponse"];
type IntegrationResponse = Schemas["integrationResponse"];
type CreateIntegrationRequest = Schemas["createIntegrationRequest"];
type UpdateIntegrationRequest = Schemas["updateIntegrationRequest"];
type NangoCredentials = Schemas["github_com_llmvault_llmvault_internal_nango.Credentials"];
type IntegConnResponse = Schemas["integConnResponse"];
type IntegConnCreateRequest = Schemas["integConnCreateRequest"];
type UsageResponse = Schemas["usageResponse"];
type AuditEntryResponse = Schemas["auditEntryResponse"];
type OrgResponse = Schemas["orgResponse"];
type ProviderSummary = Schemas["providerSummary"];
type ProviderDetail = Schemas["providerDetail"];
type ModelSummary = Schemas["modelSummary"];
type PaginatedApiKeys = Schemas["paginatedResponse-apiKeyResponse"];
type PaginatedCredentials = Schemas["paginatedResponse-credentialResponse"];
type PaginatedIdentities = Schemas["paginatedResponse-identityResponse"];
type PaginatedAuditEntries = Schemas["paginatedResponse-auditEntryResponse"];
type PaginatedIntegrations = Schemas["paginatedResponse-integrationResponse"];
type PaginatedIntegConns = Schemas["paginatedResponse-integConnResponse"];
type ErrorResponse = Schemas["errorResponse"];
type JSON = Schemas["JSON"];
type AgentResponse = Schemas["agentResponse"];
type CreateAgentRequest = Schemas["createAgentRequest"];
type UpdateAgentRequest = Schemas["updateAgentRequest"];
type PaginatedAgents = Schemas["paginatedResponse-agentResponse"];
type SandboxTemplateResponse = Schemas["sandboxTemplateResponse"];
type CreateSandboxTemplateRequest = Schemas["createSandboxTemplateRequest"];
type UpdateSandboxTemplateRequest = Schemas["updateSandboxTemplateRequest"];
type PaginatedSandboxTemplates = Schemas["paginatedResponse-sandboxTemplateResponse"];
type ConversationResponse = Schemas["conversationResponse"];
type ConversationEventResponse = Schemas["conversationEventResponse"];
type PaginatedConversations = Schemas["paginatedResponse-conversationResponse"];
type PaginatedConversationEvents = Schemas["paginatedResponse-conversationEventResponse"];
type SandboxResponse = Schemas["sandboxResponse"];
type PaginatedSandboxes = Schemas["paginatedResponse-sandboxResponse"];
type ExecRequest = Schemas["execRequest"];
type ExecResponse = Schemas["execResponse"];
type CommandResult = Schemas["commandResult"];

type ApiClient = ReturnType<typeof openapi_fetch__default<paths>>;
declare class BaseResource {
    protected client: ApiClient;
    constructor(client: ApiClient);
}

declare class AgentsResource extends BaseResource {
    create(body: CreateAgentRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["createAgentRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["agentResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        body: {
            agent_config?: components["schemas"]["JSON"];
            credential_id?: string;
            description?: string;
            identity_id?: string;
            integrations?: components["schemas"]["JSON"];
            mcp_servers?: components["schemas"]["JSON"];
            model?: string;
            name?: string;
            permissions?: components["schemas"]["JSON"];
            sandbox_template_id?: string;
            sandbox_type?: string;
            skills?: components["schemas"]["JSON"];
            subagents?: components["schemas"]["JSON"];
            system_prompt?: string;
            tools?: components["schemas"]["JSON"];
        };
    }, `${string}/${string}`>>;
    list(query?: {
        limit?: number;
        cursor?: string;
        identity_id?: string;
        status?: string;
        sandbox_type?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                identity_id?: string;
                status?: string;
                sandbox_type?: string;
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
                    "application/json": components["schemas"]["paginatedResponse-agentResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            query: {
                limit?: number;
                cursor?: string;
                identity_id?: string;
                status?: string;
                sandbox_type?: string;
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
                    "application/json": components["schemas"]["agentResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
    update(id: string, body: UpdateAgentRequest): Promise<openapi_fetch.FetchResponse<{
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
                "application/json": components["schemas"]["updateAgentRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["agentResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
            agent_config?: components["schemas"]["JSON"];
            credential_id?: string;
            description?: string;
            integrations?: components["schemas"]["JSON"];
            mcp_servers?: components["schemas"]["JSON"];
            model?: string;
            name?: string;
            permissions?: components["schemas"]["JSON"];
            sandbox_template_id?: string;
            sandbox_type?: string;
            skills?: components["schemas"]["JSON"];
            subagents?: components["schemas"]["JSON"];
            system_prompt?: string;
            tools?: components["schemas"]["JSON"];
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
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                "application/json": components["schemas"]["createAPIKeyRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["createAPIKeyResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["paginatedResponse-apiKeyResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["paginatedResponse-auditEntryResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                "application/json": components["schemas"]["createConnectSessionRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["connectSessionResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        body: {
            allowed_integrations?: string[];
            allowed_origins?: string[];
            external_id?: string;
            identity_id?: string;
            metadata?: components["schemas"]["JSON"];
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
                    "application/json": components["schemas"]["connectSettingsResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["connectSettingsResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                "application/json": components["schemas"]["connectSettingsRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["connectSettingsResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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

interface ProxyRequestOptions {
    method?: string;
    path: string;
    body?: unknown;
    query?: Record<string, string>;
    headers?: Record<string, string>;
}
interface ProxyResponse<T = unknown> {
    status: number;
    headers: Headers;
    body: T;
}
declare class ConnectionsResource extends BaseResource {
    private baseUrl;
    private apiKey;
    constructor(client: ApiClient, baseUrl: string, apiKey: string);
    availableScopes(): Promise<AvailableScopeConnection[]>;
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
                "application/json": components["schemas"]["integConnCreateRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["integConnResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
            meta?: components["schemas"]["JSON"];
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
                    "application/json": components["schemas"]["paginatedResponse-integConnResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["integConnResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
    /**
     * Proxy an arbitrary HTTP request through a connection to the upstream provider API.
     *
     * The request is forwarded via Nango with the connection's stored credentials.
     * The raw upstream response (status, headers, body) is returned as-is.
     */
    proxy<T = unknown>(id: string, options: ProxyRequestOptions): Promise<ProxyResponse<T>>;
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
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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

declare class ConversationsResource extends BaseResource {
    create(agentID: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                agentID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["conversationResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                agentID: string;
            };
        };
    }, `${string}/${string}`>>;
    list(agentID: string, query?: {
        limit?: number;
        cursor?: string;
        status?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                status?: string;
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path: {
                agentID: string;
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
                    "application/json": components["schemas"]["paginatedResponse-conversationResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                agentID: string;
            };
            query: {
                limit?: number;
                cursor?: string;
                status?: string;
            } | undefined;
        };
    }, `${string}/${string}`>>;
    get(convID: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                convID: string;
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
                    "application/json": components["schemas"]["conversationResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            410: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                convID: string;
            };
        };
    }, `${string}/${string}`>>;
    sendMessage(convID: string, content: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                convID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": {
                    content?: string;
                };
            };
        };
        responses: {
            202: {
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
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            410: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                convID: string;
            };
        };
        body: any;
    }, `${string}/${string}`>>;
    abort(convID: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                convID: string;
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
            410: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                convID: string;
            };
        };
    }, `${string}/${string}`>>;
    end(convID: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                convID: string;
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
            410: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                convID: string;
            };
        };
    }, `${string}/${string}`>>;
    listApprovals(convID: string): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                convID: string;
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
                        [key: string]: unknown;
                    }[];
                };
            };
            410: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                convID: string;
            };
        };
    }, `${string}/${string}`>>;
    resolveApproval(convID: string, requestID: string, decision: "approve" | "deny"): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path: {
                convID: string;
                requestID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": {
                    decision?: string;
                };
            };
        };
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
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            410: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                convID: string;
                requestID: string;
            };
        };
        body: any;
    }, `${string}/${string}`>>;
    listEvents(convID: string, query?: {
        limit?: number;
        cursor?: string;
        type?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                type?: string;
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path: {
                convID: string;
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
                    "application/json": components["schemas"]["paginatedResponse-conversationEventResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                convID: string;
            };
            query: {
                limit?: number;
                cursor?: string;
                type?: string;
            } | undefined;
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
                "application/json": components["schemas"]["createCredentialRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["credentialResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
            meta?: components["schemas"]["JSON"];
            provider_id?: string;
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
                    "application/json": components["schemas"]["paginatedResponse-credentialResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["credentialResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["credentialResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                "application/json": components["schemas"]["createIdentityRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["identityResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        body: {
            external_id?: string;
            meta?: components["schemas"]["JSON"];
            ratelimits?: components["schemas"]["identityRateLimitParams"][];
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
                    "application/json": components["schemas"]["paginatedResponse-identityResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["identityResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                "application/json": components["schemas"]["updateIdentityRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["identityResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
            meta?: components["schemas"]["JSON"];
            ratelimits?: components["schemas"]["identityRateLimitParams"][];
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
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                "application/json": components["schemas"]["createIntegrationRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["integrationResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            502: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        body: {
            credentials?: components["schemas"]["github_com_llmvault_llmvault_internal_nango.Credentials"];
            display_name?: string;
            meta?: components["schemas"]["JSON"];
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
                    "application/json": components["schemas"]["paginatedResponse-integrationResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["integrationResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                "application/json": components["schemas"]["updateIntegrationRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["integrationResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            502: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
            meta?: components["schemas"]["JSON"];
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
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            502: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["integrationProviderInfo"][];
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
                    "application/json": components["schemas"]["integrationProviderInfo"][];
                };
            };
        };
    }> | undefined, `${string}/${string}`>>;
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
                    "application/json": components["schemas"]["orgResponse"];
                };
            };
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["orgResponse"];
                };
            };
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["providerSummary"][];
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
                    "application/json": components["schemas"]["providerSummary"][];
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
                    "application/json": components["schemas"]["providerDetail"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["modelSummary"][];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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

declare class SandboxesResource extends BaseResource {
    list(query?: {
        limit?: number;
        cursor?: string;
        status?: string;
        identity_id?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                status?: string;
                identity_id?: string;
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
                    "application/json": components["schemas"]["paginatedResponse-sandboxResponse"];
                };
            };
        };
    }, {
        params: {
            query: {
                limit?: number;
                cursor?: string;
                status?: string;
                identity_id?: string;
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
                    "application/json": components["schemas"]["sandboxResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
    stop(id: string): Promise<openapi_fetch.FetchResponse<{
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
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
    exec(id: string, commands: string[]): Promise<openapi_fetch.FetchResponse<{
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
                "application/json": components["schemas"]["execRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["execResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            path: {
                id: string;
            };
        };
        body: ExecRequest;
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
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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

declare class SandboxTemplatesResource extends BaseResource {
    create(body: CreateSandboxTemplateRequest): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["createSandboxTemplateRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["sandboxTemplateResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        body: {
            build_commands?: string;
            config?: components["schemas"]["JSON"];
            name?: string;
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
                    "application/json": components["schemas"]["paginatedResponse-sandboxTemplateResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["sandboxTemplateResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
    update(id: string, body: UpdateSandboxTemplateRequest): Promise<openapi_fetch.FetchResponse<{
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
                "application/json": components["schemas"]["updateSandboxTemplateRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["sandboxTemplateResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
            build_commands?: string;
            config?: components["schemas"]["JSON"];
            name?: string;
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
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                "application/json": components["schemas"]["mintTokenRequest"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["mintTokenResponse"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        body: {
            credential_id?: string;
            meta?: components["schemas"]["JSON"];
            refill_amount?: number;
            refill_interval?: string;
            remaining?: number;
            scopes?: components["schemas"]["github_com_llmvault_llmvault_internal_mcp.TokenScope"][];
            ttl?: string;
        };
    }, `${string}/${string}`>>;
    list(query?: {
        limit?: number;
        cursor?: string;
        credential_id?: string;
    }): Promise<openapi_fetch.FetchResponse<{
        parameters: {
            query?: {
                limit?: number;
                cursor?: string;
                credential_id?: string;
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
                    "application/json": components["schemas"]["paginatedResponse-tokenListItem"];
                };
            };
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }, {
        params: {
            query: {
                limit?: number;
                cursor?: string;
                credential_id?: string;
            } | undefined;
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
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["usageResponse"];
                };
            };
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
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
                    "application/json": components["schemas"]["usageResponse"];
                };
            };
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["errorResponse"];
                };
            };
        };
    }> | undefined, `${string}/${string}`>>;
}

declare class LLMVault {
    readonly agents: AgentsResource;
    readonly apiKeys: ApiKeysResource;
    readonly audit: AuditResource;
    readonly connect: ConnectResource;
    readonly connections: ConnectionsResource;
    readonly conversations: ConversationsResource;
    readonly credentials: CredentialsResource;
    readonly identities: IdentitiesResource;
    readonly integrations: IntegrationsResource;
    readonly org: OrgResource;
    readonly providers: ProvidersResource;
    readonly sandboxes: SandboxesResource;
    readonly sandboxTemplates: SandboxTemplatesResource;
    readonly tokens: TokensResource;
    readonly usage: UsageResource;
    constructor(config: LLMVaultConfig);
}

export { type AgentResponse, type ApiKeyResponse, type AuditEntryResponse, type AvailableScopeAction, type AvailableScopeConnection, type AvailableScopeResource, type AvailableScopeResourceItem, type CommandResult, type ConnectSessionResponse, type ConnectSettingsRequest, type ConnectSettingsResponse, type ConversationEventResponse, type ConversationResponse, type CreateAPIKeyRequest, type CreateAPIKeyResponse, type CreateAgentRequest, type CreateConnectSessionRequest, type CreateCredentialRequest, type CreateIdentityRequest, type CreateIntegrationRequest, type CreateSandboxTemplateRequest, type CredentialResponse, type ErrorResponse, type ExecRequest, type ExecResponse, type IdentityRateLimitParams, type IdentityResponse, type IntegConnCreateRequest, type IntegConnResponse, type IntegrationResponse, type JSON, LLMVault, type LLMVaultConfig, type MintTokenRequest, type MintTokenResponse, type ModelSummary, type NangoCredentials, type OrgResponse, type PaginatedAgents, type PaginatedApiKeys, type PaginatedAuditEntries, type PaginatedConversationEvents, type PaginatedConversations, type PaginatedCredentials, type PaginatedIdentities, type PaginatedIntegConns, type PaginatedIntegrations, type PaginatedSandboxTemplates, type PaginatedSandboxes, type PaginatedTokens, type ProviderDetail, type ProviderSummary, type ProxyRequestOptions, type ProxyResponse, type SandboxResponse, type SandboxTemplateResponse, type TokenListItem, type TokenScope, type UpdateAgentRequest, type UpdateIdentityRequest, type UpdateIntegrationRequest, type UpdateSandboxTemplateRequest, type UsageResponse };
