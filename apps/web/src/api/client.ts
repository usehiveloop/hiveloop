import createFetchClient, { type Middleware } from "openapi-fetch";
import createClient from "openapi-react-query";
import type { paths } from "./schema";

const throwOnError: Middleware = {
  async onResponse({ response }) {
    if (!response.ok) {
      const body = await response.clone().json().catch(() => ({}));
      throw new Error(body.error ?? `${response.status} ${response.statusText}`);
    }
  },
};

const fetchClient = createFetchClient<paths>({ baseUrl: "/api/proxy" });
fetchClient.use(throwOnError);

/** React Query client (for use in components) */
export const $api = createClient(fetchClient);

/** Raw openapi-fetch client (for use in components) */
export { fetchClient };
