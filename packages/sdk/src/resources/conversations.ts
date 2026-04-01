import { BaseResource } from "./base.js";

export class ConversationsResource extends BaseResource {
  create(agentID: string) {
    return this.client.POST("/v1/agents/{agentID}/conversations", {
      params: { path: { agentID } },
    });
  }

  list(agentID: string, query?: { limit?: number; cursor?: string; status?: string }) {
    return this.client.GET("/v1/agents/{agentID}/conversations", {
      params: { path: { agentID }, query },
    });
  }

  get(convID: string) {
    return this.client.GET("/v1/conversations/{convID}", {
      params: { path: { convID } },
    });
  }

  sendMessage(convID: string, content: string) {
    return this.client.POST("/v1/conversations/{convID}/messages", {
      params: { path: { convID } },
      body: { content } as any,
    });
  }

  abort(convID: string) {
    return this.client.POST("/v1/conversations/{convID}/abort", {
      params: { path: { convID } },
    });
  }

  end(convID: string) {
    return this.client.DELETE("/v1/conversations/{convID}", {
      params: { path: { convID } },
    });
  }

  listApprovals(convID: string) {
    return this.client.GET("/v1/conversations/{convID}/approvals", {
      params: { path: { convID } },
    });
  }

  resolveApproval(convID: string, requestID: string, decision: "approve" | "deny") {
    return this.client.POST("/v1/conversations/{convID}/approvals/{requestID}", {
      params: { path: { convID, requestID } },
      body: { decision } as any,
    });
  }

  listEvents(convID: string, query?: { limit?: number; cursor?: string; type?: string }) {
    return this.client.GET("/v1/conversations/{convID}/events", {
      params: { path: { convID }, query },
    });
  }
}
