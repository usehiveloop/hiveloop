"use client"

import { $api } from "@/lib/api/hooks"
import { ModelCombobox } from "@/components/model-combobox"

interface ProviderModelComboboxProps {
  providerId: string
  value?: string | null
  onSelect?: (model: string) => void
}

export function ProviderModelCombobox({ providerId, value, onSelect }: ProviderModelComboboxProps) {
  const { data, isLoading } = $api.useQuery(
    "get",
    "/v1/providers/{id}/models",
    { params: { path: { id: providerId } } },
    { enabled: !!providerId },
  )

  const models = data ?? []

  return (
    <ModelCombobox
      models={models}
      value={value}
      onSelect={onSelect}
      loading={isLoading}
      disabled={isLoading || models.length === 0}
    />
  )
}
