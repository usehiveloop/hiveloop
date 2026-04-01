import { BaseResource } from "./base.js";
import type {
  CreateSandboxTemplateRequest,
  UpdateSandboxTemplateRequest,
} from "../types.js";

export class SandboxTemplatesResource extends BaseResource {
  create(body: CreateSandboxTemplateRequest) {
    return this.client.POST("/v1/sandbox-templates", { body });
  }

  list(query?: { limit?: number; cursor?: string }) {
    return this.client.GET("/v1/sandbox-templates", { params: { query } });
  }

  get(id: string) {
    return this.client.GET("/v1/sandbox-templates/{id}", {
      params: { path: { id } },
    });
  }

  update(id: string, body: UpdateSandboxTemplateRequest) {
    return this.client.PUT("/v1/sandbox-templates/{id}", {
      params: { path: { id } },
      body,
    });
  }

  delete(id: string) {
    return this.client.DELETE("/v1/sandbox-templates/{id}", {
      params: { path: { id } },
    });
  }
}
