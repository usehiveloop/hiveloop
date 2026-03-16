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
  LLMVault: () => LLMVault
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

// src/resources/credentials.ts
var CredentialsResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/credentials", { body });
  }
  list(query) {
    return this.client.GET("/v1/credentials", { params: { query } });
  }
  delete(id) {
    return this.client.DELETE("/v1/credentials/{id}", {
      params: { path: { id } }
    });
  }
};

// src/resources/tokens.ts
var TokensResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/tokens", { body });
  }
  delete(jti) {
    return this.client.DELETE("/v1/tokens/{jti}", {
      params: { path: { jti } }
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
};

// src/resources/connect.ts
var ConnectSessionsResource = class extends BaseResource {
  create(body) {
    return this.client.POST("/v1/connect/sessions", { body });
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

// src/resources/connections.ts
var ConnectionsResource = class extends BaseResource {
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
  delete(id) {
    return this.client.DELETE("/v1/connections/{id}", {
      params: { path: { id } }
    });
  }
};

// src/resources/usage.ts
var UsageResource = class extends BaseResource {
  get() {
    return this.client.GET("/v1/usage");
  }
};

// src/resources/audit.ts
var AuditResource = class extends BaseResource {
  list(query) {
    return this.client.GET("/v1/audit", { params: { query } });
  }
};

// src/resources/org.ts
var OrgResource = class extends BaseResource {
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

// src/client.ts
var LLMVault = class {
  apiKeys;
  credentials;
  tokens;
  identities;
  connect;
  integrations;
  connections;
  usage;
  audit;
  org;
  providers;
  constructor(config) {
    const client = (0, import_openapi_fetch.default)({
      baseUrl: config.baseUrl ?? "https://api.llmvault.dev",
      headers: {
        Authorization: `Bearer ${config.apiKey}`
      }
    });
    this.apiKeys = new ApiKeysResource(client);
    this.credentials = new CredentialsResource(client);
    this.tokens = new TokensResource(client);
    this.identities = new IdentitiesResource(client);
    this.connect = new ConnectResource(client);
    this.integrations = new IntegrationsResource(client);
    this.connections = new ConnectionsResource(client);
    this.usage = new UsageResource(client);
    this.audit = new AuditResource(client);
    this.org = new OrgResource(client);
    this.providers = new ProvidersResource(client);
  }
};
// Annotate the CommonJS export names for ESM import in node:
0 && (module.exports = {
  LLMVault
});
//# sourceMappingURL=index.cjs.map