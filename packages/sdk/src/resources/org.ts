import { BaseResource } from "./base.js";
import type { CreateOrgRequest } from "../types.js";

export class OrgResource extends BaseResource {
  create(body: CreateOrgRequest) {
    return this.client.POST("/v1/orgs", { body });
  }

  getCurrent() {
    return this.client.GET("/v1/orgs/current");
  }
}
