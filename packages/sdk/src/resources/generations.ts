import { BaseResource } from "./base.js";

export class GenerationsResource extends BaseResource {
  list(query?: {
    limit?: number;
    cursor?: string;
    model?: string;
    provider_id?: string;
    credential_id?: string;
    user_id?: string;
    tags?: string;
    error_type?: string;
  }) {
    return this.client.GET("/v1/generations", { params: { query } });
  }

  get(id: string) {
    return this.client.GET("/v1/generations/{id}", {
      params: { path: { id } },
    });
  }
}
