"use strict";
var __create = Object.create;
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __getProtoOf = Object.getPrototypeOf;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toESM = (mod, isNodeMode, target) => (target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(
  // If the importer is in node compatibility mode or this is not an ESM
  // file that has been converted to a CommonJS file using a Babel-
  // compatible transform (i.e. "__esModule" has not been set), then set
  // "default" to the CommonJS "module.exports" for node compatibility.
  isNodeMode || !mod || !mod.__esModule ? __defProp(target, "default", { value: mod, enumerable: true }) : target,
  mod
));
var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

// src/index.ts
var index_exports = {};
__export(index_exports, {
  HiveLoop: () => HiveLoop
});
module.exports = __toCommonJS(index_exports);

// src/client.ts
var import_openapi_fetch = __toESM(require("openapi-fetch"), 1);

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
var HiveLoop = class {
  agents;
  apiKeys;
  audit;
  catalog;
  conversations;
  credentials;
  customDomains;
  generations;
  org;
  providers;
  reporting;
  sandboxes;
  sandboxTemplates;
  tokens;
  usage;
  constructor(config) {
    const baseUrl = config.baseUrl ?? "https://api.hiveloop.com";
    const client = (0, import_openapi_fetch.default)({
      baseUrl,
      headers: {
        Authorization: `Bearer ${config.apiKey}`
      }
    });
    this.agents = new AgentsResource(client);
    this.apiKeys = new ApiKeysResource(client);
    this.audit = new AuditResource(client);
    this.catalog = new CatalogResource(client);
    this.conversations = new ConversationsResource(client, baseUrl, config.apiKey);
    this.credentials = new CredentialsResource(client);
    this.customDomains = new CustomDomainsResource(client);
    this.generations = new GenerationsResource(client);
    this.org = new OrgResource(client);
    this.providers = new ProvidersResource(client);
    this.reporting = new ReportingResource(client);
    this.sandboxes = new SandboxesResource(client);
    this.sandboxTemplates = new SandboxTemplatesResource(client);
    this.tokens = new TokensResource(client);
    this.usage = new UsageResource(client);
  }
};
// Annotate the CommonJS export names for ESM import in node:
0 && (module.exports = {
  HiveLoop
});
//# sourceMappingURL=index.cjs.map