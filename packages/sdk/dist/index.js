// src/client.ts
import createClient from "openapi-fetch";

// src/resources/base.ts
var BaseResource = class {
  constructor(client) {
    this.client = client;
  }
};

// src/resources/agents.ts
var AgentsResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/agents", { body });
  }
  list(query) {
    return this.client.GET("/v1/agents", { params: { query } });
  }
  get(id) {
    return this.client.GET("/v1/agents/{id}", {
      params: { path: { id } }
    });
  }
  update(id, body) {
    return this.client.PUT("/v1/agents/{id}", {
      params: { path: { id } },
      body
    });
  }
  delete(id) {
    return this.client.DELETE("/v1/agents/{id}", {
      params: { path: { id } }
    });
  }
  getSetup(id) {
    return this.client.GET("/v1/agents/{id}/setup", {
      params: { path: { id } }
    });
  }
  updateSetup(id, body) {
    return this.client.PUT("/v1/agents/{id}/setup", {
      params: { path: { id } },
      body
    });
  }
};

// src/resources/api-keys.ts
var ApiKeysResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/api-keys", { body });
  }
  list(query) {
    return this.client.GET("/v1/api-keys", { params: { query } });
  }
  delete(id) {
    return this.client.DELETE("/v1/api-keys/{id}", {
      params: { path: { id } }
    });
  }
};

// src/resources/audit.ts
var AuditResource = class extends BaseResource {
  list(query) {
    return this.client.GET("/v1/audit", { params: { query } });
  }
};

// src/resources/catalog.ts
var CatalogResource = class extends BaseResource {
  listIntegrations() {
    return this.client.GET("/v1/catalog/integrations");
  }
  getIntegration(id) {
    return this.client.GET("/v1/catalog/integrations/{id}", {
      params: { path: { id } }
    });
  }
  listActions(id) {
    return this.client.GET("/v1/catalog/integrations/{id}/actions", {
      params: { path: { id } }
    });
  }
};

// src/resources/connect.ts
var ConnectSessionsResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/connect/sessions", { body });
  }
  list(query) {
    return this.client.GET("/v1/connect/sessions", { params: { query } });
  }
  get(id) {
    return this.client.GET("/v1/connect/sessions/{id}", {
      params: { path: { id } }
    });
  }
  delete(id) {
    return this.client.DELETE("/v1/connect/sessions/{id}", {
      params: { path: { id } }
    });
  }
};
var ConnectSettingsResource = class extends BaseResource {
  get() {
    return this.client.GET("/v1/settings/connect");
  }
  update(body) {
    return this.client.PUT("/v1/settings/connect", { body });
  }
};
var ConnectResource = class extends BaseResource {
  sessions;
  settings;
  constructor(client) {
    super(client);
    this.sessions = new ConnectSessionsResource(client);
    this.settings = new ConnectSettingsResource(client);
  }
};

// src/resources/connections.ts
var ConnectionsResource = class extends BaseResource {
  baseUrl;
  apiKey;
  constructor(client, baseUrl, apiKey) {
    super(client);
    this.baseUrl = baseUrl;
    this.apiKey = apiKey;
  }
  async availableScopes() {
    const { data } = await this.client.GET("/v1/connections/available-scopes", {});
    return data ?? [];
  }
  create(integrationId, body) {
    return this.client.POST("/v1/integrations/{id}/connections", {
      params: { path: { id: integrationId } },
      body
    });
  }
  list(integrationId, query) {
    return this.client.GET("/v1/integrations/{id}/connections", {
      params: { path: { id: integrationId }, query }
    });
  }
  get(id) {
    return this.client.GET("/v1/connections/{id}", {
      params: { path: { id } }
    });
  }
  /**
   * Proxy an arbitrary HTTP request through a connection to the upstream provider API.
   *
   * The request is forwarded via Nango with the connection's stored credentials.
   * The raw upstream response (status, headers, body) is returned as-is.
   */
  async proxy(id, options) {
    const { method = "GET", path, body, query, headers } = options;
    let proxyPath = path.startsWith("/") ? path : `/${path}`;
    if (query && Object.keys(query).length > 0) {
      const qs = new URLSearchParams(query).toString();
      proxyPath += `?${qs}`;
    }
    const url = `${this.baseUrl}/v1/connections/${encodeURIComponent(id)}/proxy${proxyPath}`;
    const fetchHeaders = {
      Authorization: `Bearer ${this.apiKey}`,
      ...headers
    };
    const init = { method, headers: fetchHeaders };
    if (body !== void 0 && body !== null) {
      fetchHeaders["Content-Type"] = fetchHeaders["Content-Type"] ?? "application/json";
      init.body = typeof body === "string" ? body : JSON.stringify(body);
    }
    const resp = await fetch(url, init);
    const contentType = resp.headers.get("Content-Type") ?? "";
    let parsed;
    if (contentType.includes("application/json")) {
      parsed = await resp.json();
    } else {
      parsed = await resp.text();
    }
    return {
      status: resp.status,
      headers: resp.headers,
      body: parsed
    };
  }
  delete(id) {
    return this.client.DELETE("/v1/connections/{id}", {
      params: { path: { id } }
    });
  }
};

// src/resources/conversations.ts
var ConversationsResource = class extends BaseResource {
  baseUrl;
  apiKey;
  constructor(client, baseUrl, apiKey) {
    super(client);
    this.baseUrl = baseUrl;
    this.apiKey = apiKey;
  }
  create(agentID) {
    return this.client.POST("/v1/agents/{agentID}/conversations", {
      params: { path: { agentID } }
    });
  }
  list(agentID, query) {
    return this.client.GET("/v1/agents/{agentID}/conversations", {
      params: { path: { agentID }, query }
    });
  }
  get(convID) {
    return this.client.GET("/v1/conversations/{convID}", {
      params: { path: { convID } }
    });
  }
  sendMessage(convID, content) {
    return this.client.POST("/v1/conversations/{convID}/messages", {
      params: { path: { convID } },
      body: { content }
    });
  }
  abort(convID) {
    return this.client.POST("/v1/conversations/{convID}/abort", {
      params: { path: { convID } }
    });
  }
  end(convID) {
    return this.client.DELETE("/v1/conversations/{convID}", {
      params: { path: { convID } }
    });
  }
  listApprovals(convID) {
    return this.client.GET("/v1/conversations/{convID}/approvals", {
      params: { path: { convID } }
    });
  }
  resolveApproval(convID, requestID, decision) {
    return this.client.POST("/v1/conversations/{convID}/approvals/{requestID}", {
      params: { path: { convID, requestID } },
      body: { decision }
    });
  }
  listEvents(convID, query) {
    return this.client.GET("/v1/conversations/{convID}/events", {
      params: { path: { convID }, query }
    });
  }
  /**
   * Opens an SSE stream for real-time conversation events.
   * Returns the raw Response so callers can consume the ReadableStream.
   */
  async stream(convID) {
    const url = `${this.baseUrl}/v1/conversations/${encodeURIComponent(convID)}/stream`;
    return fetch(url, {
      headers: {
        Authorization: `Bearer ${this.apiKey}`,
        Accept: "text/event-stream"
      }
    });
  }
};

// src/resources/credentials.ts
var CredentialsResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/credentials", { body });
  }
  list(query) {
    return this.client.GET("/v1/credentials", { params: { query } });
  }
  get(id) {
    return this.client.GET("/v1/credentials/{id}", {
      params: { path: { id } }
    });
  }
  delete(id) {
    return this.client.DELETE("/v1/credentials/{id}", {
      params: { path: { id } }
    });
  }
};

// src/resources/custom-domains.ts
var CustomDomainsResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/custom-domains", { body });
  }
  list() {
    return this.client.GET("/v1/custom-domains");
  }
  verify(id) {
    return this.client.POST("/v1/custom-domains/{id}/verify", {
      params: { path: { id } }
    });
  }
  delete(id) {
    return this.client.DELETE("/v1/custom-domains/{id}", {
      params: { path: { id } }
    });
  }
};

// src/resources/generations.ts
var GenerationsResource = class extends BaseResource {
  list(query) {
    return this.client.GET("/v1/generations", { params: { query } });
  }
  get(id) {
    return this.client.GET("/v1/generations/{id}", {
      params: { path: { id } }
    });
  }
};

// src/resources/identities.ts
var IdentitiesResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/identities", { body });
  }
  list(query) {
    return this.client.GET("/v1/identities", { params: { query } });
  }
  get(id) {
    return this.client.GET("/v1/identities/{id}", {
      params: { path: { id } }
    });
  }
  update(id, body) {
    return this.client.PUT("/v1/identities/{id}", {
      params: { path: { id } },
      body
    });
  }
  delete(id) {
    return this.client.DELETE("/v1/identities/{id}", {
      params: { path: { id } }
    });
  }
  getSetup(id) {
    return this.client.GET("/v1/identities/{id}/setup", {
      params: { path: { id } }
    });
  }
  updateSetup(id, body) {
    return this.client.PUT("/v1/identities/{id}/setup", {
      params: { path: { id } },
      body
    });
  }
};

// src/resources/integrations.ts
var IntegrationsResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/integrations", { body });
  }
  list(query) {
    return this.client.GET("/v1/integrations", { params: { query } });
  }
  get(id) {
    return this.client.GET("/v1/integrations/{id}", {
      params: { path: { id } }
    });
  }
  update(id, body) {
    return this.client.PUT("/v1/integrations/{id}", {
      params: { path: { id } },
      body
    });
  }
  delete(id) {
    return this.client.DELETE("/v1/integrations/{id}", {
      params: { path: { id } }
    });
  }
  listProviders() {
    return this.client.GET("/v1/integrations/providers");
  }
};

// src/resources/org.ts
var OrgResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/orgs", { body });
  }
  getCurrent() {
    return this.client.GET("/v1/orgs/current");
  }
};

// src/resources/providers.ts
var ProvidersResource = class extends BaseResource {
  list() {
    return this.client.GET("/v1/providers");
  }
  get(id) {
    return this.client.GET("/v1/providers/{id}", {
      params: { path: { id } }
    });
  }
  listModels(id) {
    return this.client.GET("/v1/providers/{id}/models", {
      params: { path: { id } }
    });
  }
};

// src/resources/reporting.ts
var ReportingResource = class extends BaseResource {
  get(query) {
    return this.client.GET("/v1/reporting", { params: { query } });
  }
};

// src/resources/sandboxes.ts
var SandboxesResource = class extends BaseResource {
  list(query) {
    return this.client.GET("/v1/sandboxes", { params: { query } });
  }
  get(id) {
    return this.client.GET("/v1/sandboxes/{id}", {
      params: { path: { id } }
    });
  }
  stop(id) {
    return this.client.POST("/v1/sandboxes/{id}/stop", {
      params: { path: { id } }
    });
  }
  exec(id, commands) {
    return this.client.POST("/v1/sandboxes/{id}/exec", {
      params: { path: { id } },
      body: { commands }
    });
  }
  delete(id) {
    return this.client.DELETE("/v1/sandboxes/{id}", {
      params: { path: { id } }
    });
  }
};

// src/resources/sandbox-templates.ts
var SandboxTemplatesResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/sandbox-templates", { body });
  }
  list(query) {
    return this.client.GET("/v1/sandbox-templates", { params: { query } });
  }
  get(id) {
    return this.client.GET("/v1/sandbox-templates/{id}", {
      params: { path: { id } }
    });
  }
  update(id, body) {
    return this.client.PUT("/v1/sandbox-templates/{id}", {
      params: { path: { id } },
      body
    });
  }
  delete(id) {
    return this.client.DELETE("/v1/sandbox-templates/{id}", {
      params: { path: { id } }
    });
  }
};

// src/resources/tokens.ts
var TokensResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/tokens", { body });
  }
  list(query) {
    return this.client.GET("/v1/tokens", { params: { query } });
  }
  delete(jti) {
    return this.client.DELETE("/v1/tokens/{jti}", {
      params: { path: { jti } }
    });
  }
};

// src/resources/usage.ts
var UsageResource = class extends BaseResource {
  get() {
    return this.client.GET("/v1/usage");
  }
};

// src/client.ts
var LLMVault = class {
  agents;
  apiKeys;
  audit;
  catalog;
  connect;
  connections;
  conversations;
  credentials;
  customDomains;
  generations;
  identities;
  integrations;
  org;
  providers;
  reporting;
  sandboxes;
  sandboxTemplates;
  tokens;
  usage;
  constructor(config) {
    const baseUrl = config.baseUrl ?? "https://api.llmvault.dev";
    const client = createClient({
      baseUrl,
      headers: {
        Authorization: `Bearer ${config.apiKey}`
      }
    });
    this.agents = new AgentsResource(client);
    this.apiKeys = new ApiKeysResource(client);
    this.audit = new AuditResource(client);
    this.catalog = new CatalogResource(client);
    this.connect = new ConnectResource(client);
    this.connections = new ConnectionsResource(client, baseUrl, config.apiKey);
    this.conversations = new ConversationsResource(client, baseUrl, config.apiKey);
    this.credentials = new CredentialsResource(client);
    this.customDomains = new CustomDomainsResource(client);
    this.generations = new GenerationsResource(client);
    this.identities = new IdentitiesResource(client);
    this.integrations = new IntegrationsResource(client);
    this.org = new OrgResource(client);
    this.providers = new ProvidersResource(client);
    this.reporting = new ReportingResource(client);
    this.sandboxes = new SandboxesResource(client);
    this.sandboxTemplates = new SandboxTemplatesResource(client);
    this.tokens = new TokensResource(client);
    this.usage = new UsageResource(client);
  }
};
export {
  LLMVault
};
//# sourceMappingURL=index.js.map