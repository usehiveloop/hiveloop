import createClient from "openapi-fetch"
import type { paths } from "./schema"

export function apiUrl(path: string = "") {
  const base = process.env.NEXT_PUBLIC_API_URL!
  return `${base}${path}`
}

export const api = createClient<paths>({
  baseUrl: "/api/proxy",
})
