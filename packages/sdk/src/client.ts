import createClient from "openapi-fetch";
import type { paths } from "./generated/schema.js";
import type { LLMVaultConfig } from "./types.js";
import { AgentsResource } from "./resources/agents.js";
import { ApiKeysResource } from "./resources/api-keys.js";
import { AuditResource } from "./resources/audit.js";
import { ConnectResource } from "./resources/connect.js";
import { ConnectionsResource } from "./resources/connections.js";
import { ConversationsResource } from "./resources/conversations.js";
import { CredentialsResource } from "./resources/credentials.js";
import { IdentitiesResource } from "./resources/identities.js";
import { IntegrationsResource } from "./resources/integrations.js";
import { OrgResource } from "./resources/org.js";
import { ProvidersResource } from "./resources/providers.js";
import { SandboxesResource } from "./resources/sandboxes.js";
import { SandboxTemplatesResource } from "./resources/sandbox-templates.js";
import { TokensResource } from "./resources/tokens.js";
import { UsageResource } from "./resources/usage.js";

export class LLMVault {
  public readonly agents: AgentsResource;
  public readonly apiKeys: ApiKeysResource;
  public readonly audit: AuditResource;
  public readonly connect: ConnectResource;
  public readonly connections: ConnectionsResource;
  public readonly conversations: ConversationsResource;
  public readonly credentials: CredentialsResource;
  public readonly identities: IdentitiesResource;
  public readonly integrations: IntegrationsResource;
  public readonly org: OrgResource;
  public readonly providers: ProvidersResource;
  public readonly sandboxes: SandboxesResource;
  public readonly sandboxTemplates: SandboxTemplatesResource;
  public readonly tokens: TokensResource;
  public readonly usage: UsageResource;

  constructor(config: LLMVaultConfig) {
    const baseUrl = config.baseUrl ?? "https://api.llmvault.dev";
    const client = createClient<paths>({
      baseUrl,
      headers: {
        Authorization: `Bearer ${config.apiKey}`,
      },
    });

    this.agents = new AgentsResource(client);
    this.apiKeys = new ApiKeysResource(client);
    this.audit = new AuditResource(client);
    this.connect = new ConnectResource(client);
    this.connections = new ConnectionsResource(client, baseUrl, config.apiKey);
    this.conversations = new ConversationsResource(client);
    this.credentials = new CredentialsResource(client);
    this.identities = new IdentitiesResource(client);
    this.integrations = new IntegrationsResource(client);
    this.org = new OrgResource(client);
    this.providers = new ProvidersResource(client);
    this.sandboxes = new SandboxesResource(client);
    this.sandboxTemplates = new SandboxTemplatesResource(client);
    this.tokens = new TokensResource(client);
    this.usage = new UsageResource(client);
  }
}
