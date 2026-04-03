import { BaseResource } from "./base.js";

export class WebhooksResource extends BaseResource {
  get() {
    return this.client.GET("/v1/settings/webhooks");
  }

  update(body: { url: string }) {
    return this.client.PUT("/v1/settings/webhooks", { body });
  }

  rotateSecret() {
    return this.client.POST("/v1/settings/webhooks/rotate-secret");
  }

  delete() {
    return this.client.DELETE("/v1/settings/webhooks");
  }
}