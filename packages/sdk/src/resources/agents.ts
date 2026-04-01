import { BaseResource } from "./base.js";
import type { CreateAgentRequest, UpdateAgentRequest } from "../types.js";

export class AgentsResource extends BaseResource {
  create(body: CreateAgentRequest) {
    return this.client.POST("/v1/agents", { body });
  }

  list(query?: {
    limit?: number;
    cursor?: string;
    identity_id?: string;
    status?: string;
    sandbox_type?: string;
  }) {
    return this.client.GET("/v1/agents", { params: { query } });
  }

  get(id: string) {
    return this.client.GET("/v1/agents/{id}", {
      params: { path: { id } },
    });
  }

  update(id: string, body: UpdateAgentRequest) {
    return this.client.PUT("/v1/agents/{id}", {
      params: { path: { id } },
      body,
    });
  }

  delete(id: string) {
    return this.client.DELETE("/v1/agents/{id}", {
      params: { path: { id } },
    });
  }
}
