import type { IntegrationResponse, NangoProvider, PaginatedResponse } from "./utils";

const BASE = "/api/proxy/v1";

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `${res.status} ${res.statusText}`);
  }
  return res.json();
}

export async function listProviders(): Promise<NangoProvider[]> {
  const res = await fetch(`${BASE}/integrations/providers`);
  return handleResponse(res);
}

export async function listIntegrations(params?: {
  limit?: number;
  cursor?: string;
  provider?: string;
}): Promise<PaginatedResponse<IntegrationResponse>> {
  const query = new URLSearchParams();
  if (params?.limit) query.set("limit", String(params.limit));
  if (params?.cursor) query.set("cursor", params.cursor);
  if (params?.provider) query.set("provider", params.provider);
  const qs = query.toString();
  const res = await fetch(`${BASE}/integrations${qs ? `?${qs}` : ""}`);
  return handleResponse(res);
}

export async function getIntegration(id: string): Promise<IntegrationResponse> {
  const res = await fetch(`${BASE}/integrations/${id}`);
  return handleResponse(res);
}

export async function createIntegration(body: {
  provider: string;
  display_name: string;
  credentials?: Record<string, unknown>;
  meta?: Record<string, unknown>;
}): Promise<IntegrationResponse> {
  const res = await fetch(`${BASE}/integrations`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return handleResponse(res);
}

export async function updateIntegration(
  id: string,
  body: {
    display_name?: string;
    credentials?: Record<string, unknown>;
    meta?: Record<string, unknown>;
  },
): Promise<IntegrationResponse> {
  const res = await fetch(`${BASE}/integrations/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return handleResponse(res);
}

export async function deleteIntegration(id: string): Promise<void> {
  const res = await fetch(`${BASE}/integrations/${id}`, { method: "DELETE" });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `${res.status} ${res.statusText}`);
  }
}
