import { BaseResource, type ApiClient } from "./base.js";

export class ConversationsResource extends BaseResource {
  private baseUrl: string;
  private apiKey: string;

  constructor(client: ApiClient, baseUrl: string, apiKey: string) {
    super(client);
    this.baseUrl = baseUrl;
    this.apiKey = apiKey;
  }

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
      body: { content },
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
      body: { decision },
    });
  }

  listEvents(convID: string, query?: { limit?: number; cursor?: string; type?: string }) {
    return this.client.GET("/v1/conversations/{convID}/events", {
      params: { path: { convID }, query },
    });
  }

  /**
   * Opens an SSE stream for real-time conversation events.
   * Returns the raw Response so callers can consume the ReadableStream.
   */
  async stream(convID: string): Promise<Response> {
    const url = `${this.baseUrl}/v1/conversations/${encodeURIComponent(convID)}/stream`;
    return fetch(url, {
      headers: {
        Authorization: `Bearer ${this.apiKey}`,
        Accept: "text/event-stream",
      },
    });
  }
}
