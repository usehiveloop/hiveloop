import { BaseResource, type ApiClient } from "./base.js";
import type { IntegConnCreateRequest, AvailableScopeConnection } from "../types.js";

export interface ProxyRequestOptions {
  method?: string;
  path: string;
  body?: unknown;
  query?: Record<string, string>;
  headers?: Record<string, string>;
}

export interface ProxyResponse<T = unknown> {
  status: number;
  headers: Headers;
  body: T;
}

export class ConnectionsResource extends BaseResource {
  private baseUrl: string;
  private apiKey: string;

  constructor(client: ApiClient, baseUrl: string, apiKey: string) {
    super(client);
    this.baseUrl = baseUrl;
    this.apiKey = apiKey;
  }

  async availableScopes(): Promise<AvailableScopeConnection[]> {
    const { data } = await this.client.GET("/v1/connections/available-scopes", {});
    return (data as unknown as AvailableScopeConnection[]) ?? [];
  }

  create(integrationId: string, body: IntegConnCreateRequest) {
    return this.client.POST("/v1/integrations/{id}/connections", {
      params: { path: { id: integrationId } },
      body,
    });
  }

  list(integrationId: string, query?: { limit?: number; cursor?: string }) {
    return this.client.GET("/v1/integrations/{id}/connections", {
      params: { path: { id: integrationId }, query },
    });
  }

  get(id: string) {
    return this.client.GET("/v1/connections/{id}", {
      params: { path: { id } },
    });
  }

  /**
   * Proxy an arbitrary HTTP request through a connection to the upstream provider API.
   *
   * The request is forwarded via Nango with the connection's stored credentials.
   * The raw upstream response (status, headers, body) is returned as-is.
   */
  async proxy<T = unknown>(id: string, options: ProxyRequestOptions): Promise<ProxyResponse<T>> {
    const { method = "GET", path, body, query, headers } = options;

    let proxyPath = path.startsWith("/") ? path : `/${path}`;

    if (query && Object.keys(query).length > 0) {
      const qs = new URLSearchParams(query).toString();
      proxyPath += `?${qs}`;
    }

    const url = `${this.baseUrl}/v1/connections/${encodeURIComponent(id)}/proxy${proxyPath}`;

    const fetchHeaders: Record<string, string> = {
      Authorization: `Bearer ${this.apiKey}`,
      ...headers,
    };

    const init: RequestInit = { method, headers: fetchHeaders };

    if (body !== undefined && body !== null) {
      fetchHeaders["Content-Type"] = fetchHeaders["Content-Type"] ?? "application/json";
      init.body = typeof body === "string" ? body : JSON.stringify(body);
    }

    const resp = await fetch(url, init);

    const contentType = resp.headers.get("Content-Type") ?? "";
    let parsed: T;
    if (contentType.includes("application/json")) {
      parsed = (await resp.json()) as T;
    } else {
      parsed = (await resp.text()) as T;
    }

    return {
      status: resp.status,
      headers: resp.headers,
      body: parsed,
    };
  }

  delete(id: string) {
    return this.client.DELETE("/v1/connections/{id}", {
      params: { path: { id } },
    });
  }
}
