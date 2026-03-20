import { BaseResource } from "./base.js";
import type { CreateCredentialRequest } from "../types.js";

export class CredentialsResource extends BaseResource {
  create(body: CreateCredentialRequest) {
    return this.client.POST("/v1/credentials", { body });
  }

  list(query?: {
    limit?: number;
    cursor?: string;
    identity_id?: string;
    external_id?: string;
    meta?: string;
  }) {
    return this.client.GET("/v1/credentials", { params: { query } });
  }

  get(id: string) {
    return this.client.GET("/v1/credentials/{id}", {
      params: { path: { id } },
    });
  }

  delete(id: string) {
    return this.client.DELETE("/v1/credentials/{id}", {
      params: { path: { id } },
    });
  }
}
