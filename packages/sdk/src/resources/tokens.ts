import { BaseResource } from "./base.js";
import type { MintTokenRequest } from "../types.js";

export class TokensResource extends BaseResource {
  create(body: MintTokenRequest) {
    return this.client.POST("/v1/tokens", { body });
  }

  list(query?: { limit?: number; cursor?: string; credential_id?: string }) {
    return this.client.GET("/v1/tokens", { params: { query } });
  }

  delete(jti: string) {
    return this.client.DELETE("/v1/tokens/{jti}", {
      params: { path: { jti } },
    });
  }
}
