"use client"

import { useQuery, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api/client"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

export type SandboxTemplate = components["schemas"]["sandboxTemplateResponse"]

interface UseSandboxTemplateOptions {
  enabled?: boolean
}

export function useSandboxTemplate(
  templateId: string | null,
  options: UseSandboxTemplateOptions = {}
) {
  const { enabled = true } = options

  return useQuery({
    queryKey: ["sandbox-template", templateId],
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
  return useQuery({
    queryKey: ["sandbox-templates"],
    queryFn: async () => {
      const response = await api.GET("/v1/sandbox-templates")
      if (response.error) {
        throw response.error
      }
      return (response.data?.data ?? []) as SandboxTemplate[]
    },
  })
}

export function useTriggerBuild() {
  const queryClient = useQueryClient()

  return $api.useMutation("post", "/v1/sandbox-templates/{id}/build", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sandbox-templates"] })
      queryClient.invalidateQueries({
        queryKey: ["sandbox-template"],
        exact: false,
      })
    },
  })
}

export function useDeleteSandboxTemplate() {
  const queryClient = useQueryClient()

  return $api.useMutation("delete", "/v1/sandbox-templates/{id}", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sandbox-templates"] })
    },
  })
}

export async function createSandboxTemplate(data: {
  name: string
  build_commands: string
}): Promise<SandboxTemplate> {
  const response = await api.POST("/v1/sandbox-templates", {
    body: data,
  })
  if (response.error) {
    throw response.error
  }
  return response.data as SandboxTemplate
}
