"use client"

import { $api } from "@/lib/api/hooks"
import { ModelCombobox } from "@/components/model-combobox"

interface AllModelsComboboxProps {
  value?: string | null
  onSelect?: (model: string) => void
}

export function AllModelsCombobox({ value, onSelect }: AllModelsComboboxProps) {
  const { data, isLoading } = $api.useQuery("get", "/v1/models", {})

  const models = data ?? []

  return (
    <ModelCombobox
      models={models}
      value={value}
      onSelect={onSelect}
      loading={isLoading}
      disabled={isLoading}
    />
  )
}
