import { BaseResource } from "./base.js";
import type { CreateDomainRequest } from "../types.js";

export class CustomDomainsResource extends BaseResource {
  create(body: CreateDomainRequest) {
    return this.client.POST("/v1/custom-domains", { body });
  }

  list() {
    return this.client.GET("/v1/custom-domains");
  }

  verify(id: string) {
    return this.client.POST("/v1/custom-domains/{id}/verify", {
      params: { path: { id } },
    });
  }

  delete(id: string) {
    return this.client.DELETE("/v1/custom-domains/{id}", {
      params: { path: { id } },
    });
  }
}
