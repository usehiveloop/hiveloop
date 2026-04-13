"use client"

import { useQuery, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api/client"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

export type SandboxTemplate = components["schemas"]["sandboxTemplateResponse"]

// Uses manual useQuery because $api.useQuery doesn't support refetchInterval as a function,
// which is needed to poll only while the template is building.
export function useSandboxTemplate(
  templateId: string | null,
  options: { enabled?: boolean } = {}
) {
  const { enabled = true } = options

  return useQuery({
    queryKey: ["get", "/v1/sandbox-templates/{id}", { params: { path: { id: templateId } } }],
    queryFn: async () => {
      if (!templateId) return null
      const response = await api.GET("/v1/sandbox-templates/{id}", {
        params: { path: { id: templateId } },
      })
      if (response.error) {
        throw response.error
      }
      return response.data as SandboxTemplate
    },
    enabled: enabled && templateId !== null,
    refetchInterval: (query) => {
      const data = query.state.data
      if (!data) return false
      if (data.build_status === "ready" || data.build_status === "failed") {
        return false
      }
      return 3000
    },
    refetchIntervalInBackground: false,
  })
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

export async function createSandboxTemplate(data: {
  name: string
  build_commands: string[]
  base_template_id?: string
}): Promise<SandboxTemplate> {
  const response = await api.POST("/v1/sandbox-templates", {
    body: data,
  })
  if (response.error) {
    throw response.error
  }
  return response.data as SandboxTemplate
}
