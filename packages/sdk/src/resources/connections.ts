import { BaseResource } from "./base.js";
import type { IntegConnCreateRequest, AvailableScopeConnection } from "../types.js";

export class ConnectionsResource extends BaseResource {
  async availableScopes(): Promise<AvailableScopeConnection[]> {
    const { data } = await this.client.GET("/v1/connections/available-scopes" as any, {});
    return (data as unknown as AvailableScopeConnection[]) ?? [];
  }

  create(integrationId: string, body: IntegConnCreateRequest) {
    return this.client.POST("/v1/integrations/{id}/connections", {
      params: { path: { id: integrationId } },
      body,
    });
  }

  list(integrationId: string, query?: { limit?: number; cursor?: string }) {
    return this.client.GET("/v1/integrations/{id}/connections", {
      params: { path: { id: integrationId }, query },
    });
  }

  get(id: string) {
    return this.client.GET("/v1/connections/{id}", {
      params: { path: { id } },
    });
  }

  delete(id: string) {
    return this.client.DELETE("/v1/connections/{id}", {
      params: { path: { id } },
    });
  }
}
