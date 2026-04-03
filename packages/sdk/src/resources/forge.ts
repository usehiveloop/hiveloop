import { BaseResource, type ApiClient } from "./base.js";

export class ForgeResource extends BaseResource {
  private baseUrl: string;
  private apiKey: string;

  constructor(client: ApiClient, baseUrl: string, apiKey: string) {
    super(client);
    this.baseUrl = baseUrl;
    this.apiKey = apiKey;
  }

  start(agentID: string, body: {
    architect_credential_id: string;
    architect_model: string;
    eval_designer_credential_id: string;
    eval_designer_model: string;
    judge_credential_id: string;
    judge_model: string;
    max_iterations?: number;
    pass_threshold?: number;
    convergence_limit?: number;
  }) {
    return this.client.POST("/v1/agents/{agentID}/forge", {
      params: { path: { agentID } },
      body,
    });
  }

  listRuns(agentID: string) {
    return this.client.GET("/v1/agents/{agentID}/forge", {
      params: { path: { agentID } },
    });
  }

  getRun(runID: string) {
    return this.client.GET("/v1/forge-runs/{runID}", {
      params: { path: { runID } },
    });
  }

  listEvents(runID: string) {
    return this.client.GET("/v1/forge-runs/{runID}/events", {
      params: { path: { runID } },
    });
  }

  listEvals(runID: string, iterationID: string) {
    return this.client.GET("/v1/forge-runs/{runID}/iterations/{iterationID}/evals", {
      params: { path: { runID, iterationID } },
    });
  }

  cancel(runID: string) {
    return this.client.POST("/v1/forge-runs/{runID}/cancel", {
      params: { path: { runID } },
    });
  }

  apply(runID: string) {
    return this.client.POST("/v1/forge-runs/{runID}/apply", {
      params: { path: { runID } },
    });
  }

  /**
   * Opens an SSE stream for real-time forge events.
   * Returns the raw Response so callers can consume the ReadableStream.
   */
  async stream(runID: string): Promise<Response> {
    const url = `${this.baseUrl}/v1/forge-runs/${encodeURIComponent(runID)}/stream`;
    return fetch(url, {
      headers: {
        Authorization: `Bearer ${this.apiKey}`,
        Accept: "text/event-stream",
      },
    });
  }
}