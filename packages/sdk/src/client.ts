import createClient from "openapi-fetch";
import type { paths } from "./generated/schema.js";
import type { LLMVaultConfig } from "./types.js";
import { ApiKeysResource } from "./resources/api-keys.js";
import { CredentialsResource } from "./resources/credentials.js";
import { TokensResource } from "./resources/tokens.js";
import { IdentitiesResource } from "./resources/identities.js";
import { ConnectResource } from "./resources/connect.js";
import { IntegrationsResource } from "./resources/integrations.js";
import { ConnectionsResource } from "./resources/connections.js";
import { UsageResource } from "./resources/usage.js";
import { AuditResource } from "./resources/audit.js";
import { OrgResource } from "./resources/org.js";
import { ProvidersResource } from "./resources/providers.js";

export class LLMVault {
  public readonly apiKeys: ApiKeysResource;
  public readonly credentials: CredentialsResource;
  public readonly tokens: TokensResource;
  public readonly identities: IdentitiesResource;
  public readonly connect: ConnectResource;
  public readonly integrations: IntegrationsResource;
  public readonly connections: ConnectionsResource;
  public readonly usage: UsageResource;
  public readonly audit: AuditResource;
  public readonly org: OrgResource;
  public readonly providers: ProvidersResource;

  constructor(config: LLMVaultConfig) {
    const baseUrl = config.baseUrl ?? "https://api.llmvault.dev";
    const client = createClient<paths>({
      baseUrl,
      headers: {
        Authorization: `Bearer ${config.apiKey}`,
      },
    });

    this.apiKeys = new ApiKeysResource(client);
    this.credentials = new CredentialsResource(client);
    this.tokens = new TokensResource(client);
    this.identities = new IdentitiesResource(client);
    this.connect = new ConnectResource(client);
    this.integrations = new IntegrationsResource(client);
    this.connections = new ConnectionsResource(client, baseUrl, config.apiKey);
    this.usage = new UsageResource(client);
    this.audit = new AuditResource(client);
    this.org = new OrgResource(client);
    this.providers = new ProvidersResource(client);
  }
}
