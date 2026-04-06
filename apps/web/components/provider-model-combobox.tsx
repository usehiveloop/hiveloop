"use client"

import { useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { ModelCombobox } from "@/components/model-combobox"

interface ProviderModelComboboxProps {
  providerId: string
  value?: string | null
  onSelect?: (model: string) => void
}

export function ProviderModelCombobox({ providerId, value, onSelect }: ProviderModelComboboxProps) {
  const { data: modelsData, isLoading } = $api.useQuery(
    "get",
    "/v1/providers/{id}/models",
    { params: { path: { id: providerId } } },
    { enabled: !!providerId },
  )

  const modelIds = useMemo(() => {
    return (modelsData ?? []).map((item) => item.id ?? "").filter(Boolean)
  }, [modelsData])

  return (
    <ModelCombobox
      models={modelIds}
      value={value}
      onSelect={onSelect}
      loading={isLoading}
      disabled={isLoading || modelIds.length === 0}
    />
  )
}
