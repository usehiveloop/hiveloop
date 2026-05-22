"use client"

import { $api } from "@/lib/api/hooks"

export function useCurrentUser() {
  const query = $api.useQuery("get", "/auth/me", {}, { retry: false })

  return {
    ...query,
    user: query.data?.user ?? null,
    orgs: query.data?.orgs ?? [],
    isPlatformAdmin: query.data?.is_platform_admin === true,
  }
}
