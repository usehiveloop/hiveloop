import createClient from "openapi-fetch";
import type { paths } from "./generated/schema.js";
import type { HiveLoopConfig } from "./types.js";
import { AgentsResource } from "./resources/agents.js";
import { ApiKeysResource } from "./resources/api-keys.js";
import { AuditResource } from "./resources/audit.js";
import { CatalogResource } from "./resources/catalog.js";
import { ConversationsResource } from "./resources/conversations.js";
import { CredentialsResource } from "./resources/credentials.js";
import { CustomDomainsResource } from "./resources/custom-domains.js";
import { GenerationsResource } from "./resources/generations.js";
import { OrgResource } from "./resources/org.js";
import { ProvidersResource } from "./resources/providers.js";
import { ReportingResource } from "./resources/reporting.js";
import { SandboxesResource } from "./resources/sandboxes.js";
import { SandboxTemplatesResource } from "./resources/sandbox-templates.js";
import { TokensResource } from "./resources/tokens.js";
import { UsageResource } from "./resources/usage.js";

export class HiveLoop {
  public readonly agents: AgentsResource;
  public readonly apiKeys: ApiKeysResource;
  public readonly audit: AuditResource;
  public readonly catalog: CatalogResource;
  public readonly conversations: ConversationsResource;
  public readonly credentials: CredentialsResource;
  public readonly customDomains: CustomDomainsResource;
  public readonly generations: GenerationsResource;
  public readonly org: OrgResource;
  public readonly providers: ProvidersResource;
  public readonly reporting: ReportingResource;
  public readonly sandboxes: SandboxesResource;
  public readonly sandboxTemplates: SandboxTemplatesResource;
  public readonly tokens: TokensResource;
  public readonly usage: UsageResource;

  constructor(config: HiveLoopConfig) {
    const baseUrl = config.baseUrl ?? "https://api.hiveloop.com";
    const client = createClient<paths>({
      baseUrl,
      headers: {
        Authorization: `Bearer ${config.apiKey}`,
      },
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
}
