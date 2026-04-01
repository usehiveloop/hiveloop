import { BaseResource } from "./base.js";
import type { ExecRequest } from "../types.js";

export class SandboxesResource extends BaseResource {
  list(query?: { limit?: number; cursor?: string; status?: string; identity_id?: string }) {
    return this.client.GET("/v1/sandboxes", { params: { query } });
  }

  get(id: string) {
    return this.client.GET("/v1/sandboxes/{id}", {
      params: { path: { id } },
    });
  }

  stop(id: string) {
    return this.client.POST("/v1/sandboxes/{id}/stop", {
      params: { path: { id } },
    });
  }

  exec(id: string, commands: string[]) {
    return this.client.POST("/v1/sandboxes/{id}/exec", {
      params: { path: { id } },
      body: { commands } as ExecRequest,
    });
  }

  delete(id: string) {
    return this.client.DELETE("/v1/sandboxes/{id}", {
      params: { path: { id } },
    });
  }
}
