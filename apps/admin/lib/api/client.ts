import createClient from "openapi-fetch"
import type { paths } from "./schema"

export function apiUrl(path: string = "") {
  const base = process.env.API_URL as string
  return `${base}${path}`
}

export const api = createClient<paths>({
  baseUrl: "/api/proxy",
})
