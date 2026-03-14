import createFetchClient from "openapi-fetch";
import { getAccessToken } from "@logto/next/server-actions";
import { getLogtoConfig } from "@/lib/logto";
import { getSelectedOrgId } from "@/lib/org";
import type { paths } from "./schema";

const API_URL = process.env.NEXT_PUBLIC_API_URL!;

/**
 * Server-side openapi-fetch client that calls the backend directly
 * with the user's Logto access token. For use in Route Handlers
 * and Server Actions (not Server Components).
 */
export function createServerClient() {
  const client = createFetchClient<paths>({ baseUrl: API_URL });

  client.use({
    async onRequest({ request }) {
      const config = getLogtoConfig();
      const resource = config.resources?.[0];
      const orgId = await getSelectedOrgId();
      const token = await getAccessToken(config, resource, orgId);
      if (token) {
        request.headers.set("Authorization", `Bearer ${token}`);
      }
      return request;
    },
  });

  return client;
}
