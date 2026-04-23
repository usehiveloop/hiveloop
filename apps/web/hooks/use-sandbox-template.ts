"use client"

import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

export type SandboxTemplate = components["schemas"]["sandboxTemplateResponse"]

// Uses $api.useQuery with a custom refetchInterval function to poll only while
// the template is building. react-query accepts the function form regardless of
// the typed wrapper, so we cast to satisfy openapi-react-query's stricter typing.
export function useSandboxTemplate(
  templateId: string | null,
  options: { enabled?: boolean } = {}
) {
  const { enabled = true } = options

  return $api.useQuery(
    "get",
    "/v1/sandbox-templates/{id}",
    { params: { path: { id: templateId ?? "" } } },
    {
      enabled: enabled && templateId !== null,
      refetchInterval: ((query: { state: { data?: SandboxTemplate } }) => {
        const data = query.state.data
        if (!data) return false
        if (data.build_status === "ready" || data.build_status === "failed") {
          return false
        }
        return 3000
      }) as unknown as number | false,
      refetchIntervalInBackground: false,
    },
  )
}

export function useSandboxTemplates() {
  return $api.useQuery("get", "/v1/sandbox-templates")
}

export function useTriggerBuild() {
  const queryClient = useQueryClient()

  return $api.useMutation("post", "/v1/sandbox-templates/{id}/build", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/sandbox-templates"] })
    },
  })
}

export function useDeleteSandboxTemplate() {
  const queryClient = useQueryClient()

  return $api.useMutation("delete", "/v1/sandbox-templates/{id}", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/sandbox-templates"] })
    },
  })
}

export function useRetryBuild() {
  const queryClient = useQueryClient()

  return $api.useMutation("post", "/v1/sandbox-templates/{id}/retry", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/sandbox-templates"] })
    },
  })
}

export function usePublicTemplates() {
  return $api.useQuery("get", "/v1/sandbox-templates/public")
}

export type PublicTemplate = components["schemas"]["publicTemplateResponse"]

export function useCreateSandboxTemplate() {
  const queryClient = useQueryClient()

  return $api.useMutation("post", "/v1/sandbox-templates", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/sandbox-templates"] })
    },
  })
}
