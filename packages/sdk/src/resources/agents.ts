import { BaseResource } from "./base.js";

export class AgentsResource extends BaseResource {
  listBuiltInTools() {
    return this.client.GET("/v1/agents/built-in-tools");
  }

  listCategories() {
    return this.client.GET("/v1/agents/categories");
  }

  listSandboxTools() {
    return this.client.GET("/v1/agents/sandbox-tools");
  }
}
