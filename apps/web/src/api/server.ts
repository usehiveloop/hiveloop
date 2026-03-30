import createFetchClient from "openapi-fetch";
import { getAccessToken } from "@/lib/auth";
import type { paths } from "./schema";

const API_URL = process.env.NEXT_PUBLIC_API_URL!;

/**
 * Server-side openapi-fetch client that calls the backend directly
 * with the user's access token from cookies. For use in Route Handlers
 * and Server Actions (not Server Components).
 */
export function createServerClient() {
  const client = createFetchClient<paths>({ baseUrl: API_URL });

  client.use({
    async onRequest({ request }) {
      const token = await getAccessToken();
      if (token) {
        request.headers.set("Authorization", `Bearer ${token}`);
      }
      return request;
    },
  });

  return client;
}
