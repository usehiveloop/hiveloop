import { BaseResource } from "./base.js";

export class CatalogResource extends BaseResource {
  listIntegrations() {
    return this.client.GET("/v1/catalog/integrations");
  }

  getIntegration(id: string) {
    return this.client.GET("/v1/catalog/integrations/{id}", {
      params: { path: { id } },
    });
  }

  listActions(id: string) {
    return this.client.GET("/v1/catalog/integrations/{id}/actions", {
      params: { path: { id } },
    });
  }
}
