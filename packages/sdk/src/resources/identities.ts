import { BaseResource } from "./base.js";
import type {
  CreateIdentityRequest,
  UpdateIdentityRequest,
  SetupRequest,
} from "../types.js";

export class IdentitiesResource extends BaseResource {
  create(body: CreateIdentityRequest) {
    return this.client.POST("/v1/identities", { body });
  }

  list(query?: {
    limit?: number;
    cursor?: string;
    external_id?: string;
    meta?: string;
  }) {
    return this.client.GET("/v1/identities", { params: { query } });
  }

  get(id: string) {
    return this.client.GET("/v1/identities/{id}", {
      params: { path: { id } },
    });
  }

  update(id: string, body: UpdateIdentityRequest) {
    return this.client.PUT("/v1/identities/{id}", {
      params: { path: { id } },
      body,
    });
  }

  delete(id: string) {
    return this.client.DELETE("/v1/identities/{id}", {
      params: { path: { id } },
    });
  }

  getSetup(id: string) {
    return this.client.GET("/v1/identities/{id}/setup", {
      params: { path: { id } },
    });
  }

  updateSetup(id: string, body: SetupRequest) {
    return this.client.PUT("/v1/identities/{id}/setup", {
      params: { path: { id } },
      body,
    });
  }
}
