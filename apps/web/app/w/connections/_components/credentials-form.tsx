"use client"

import { useState, useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, Loading03Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { IntegrationLogo } from "@/components/integration-logo"
import type { components } from "@/lib/api/schema"

type NangoConfig = components["schemas"]["NangoConfig"]
type ConnectionConfigField = components["schemas"]["ConnectionConfigField"]

interface Integration {
  id?: string
  provider?: string
  display_name?: string
  nango_config?: NangoConfig
}

interface CredentialsFormProps {
  integration: Integration
  onSubmit: (
    credentials: Record<string, string> | undefined,
    params: Record<string, string>,
    installation?: "outbound",
  ) => void
  onBack: () => void
  isSubmitting: boolean
}

interface FieldConfig {
  key: string
  title: string
  description: string
  placeholder: string
  pattern?: string
  type: "text" | "password"
}

function getConnectionConfigFields(
  connectionConfig: Record<string, ConnectionConfigField> | undefined,
): FieldConfig[] {
  if (!connectionConfig) return []

  return Object.entries(connectionConfig)
    .filter(([, field]) => !field.automated)
    .map(([key, field]) => ({
      key,
      title: field.title || key,
      description: field.description || "",
      placeholder: field.example || "",
      pattern: field.pattern,
      type: "text" as const,
    }))
}

export function CredentialsForm({
  integration,
  onSubmit,
  onBack,
  isSubmitting,
}: CredentialsFormProps) {
  const authMode = integration.nango_config?.auth_mode
  const installation = integration.nango_config?.installation
  const isApiKey = authMode === "API_KEY"
  const isBasic = authMode === "BASIC"
  const isOutbound = installation === "outbound"

  const configFields = useMemo(
    () => getConnectionConfigFields(integration.nango_config?.connection_config as Record<string, ConnectionConfigField> | undefined),
    [integration.nango_config?.connection_config],
  )

  const [apiKey, setApiKey] = useState("")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [configValues, setConfigValues] = useState<Record<string, string>>({})

  function setConfigValue(key: string, value: string) {
    setConfigValues((prev) => ({ ...prev, [key]: value }))
  }

  function isValid(): boolean {
    if (isApiKey && !apiKey.trim()) return false
    if (isBasic && (!username.trim() || !password.trim())) return false

    for (const field of configFields) {
      const value = configValues[field.key] ?? ""
      if (!value.trim()) return false
      if (field.pattern) {
        try {
          if (!new RegExp(field.pattern).test(value)) return false
        } catch {
          // skip invalid patterns
        }
      }
    }

    return true
  }

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    if (!isValid()) return

    let credentials: Record<string, string> | undefined
    if (isApiKey) {
      credentials = { apiKey: apiKey.trim() }
    } else if (isBasic) {
      credentials = { username: username.trim(), password: password.trim() }
    } else if (isOutbound) {
      credentials = {}
    }

    const params: Record<string, string> = {}
    for (const field of configFields) {
      const value = configValues[field.key]?.trim()
      if (value) params[field.key] = value
    }

    onSubmit(credentials, params, isOutbound ? "outbound" : undefined)
  }

  const description = isApiKey
    ? `Enter your API key to connect ${integration.display_name}.`
    : isOutbound
      ? `Enter the required details, then complete the installation from ${integration.display_name}'s settings.`
      : `Enter the required details to connect ${integration.display_name}.`

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 mb-1">
        <button
          type="button"
          onClick={onBack}
          className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1 cursor-pointer"
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
        </button>
        <IntegrationLogo provider={integration.provider ?? ""} size={20} />
        <h3 className="font-heading text-base font-medium leading-none">
          {integration.display_name}
        </h3>
      </div>
      <p className="text-sm text-muted-foreground mb-5">{description}</p>

      <form onSubmit={handleSubmit} className="flex flex-col gap-4 flex-1">
        {isApiKey && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="conn-api-key">API key</Label>
            <Input
              id="conn-api-key"
              type="password"
              value={apiKey}
              onChange={(event) => setApiKey(event.target.value)}
              placeholder="Enter your API key"
              required
              autoFocus
            />
          </div>
        )}

        {isBasic && (
          <>
            <div className="flex flex-col gap-2">
              <Label htmlFor="conn-username">Username</Label>
              <Input
                id="conn-username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                placeholder="Enter username"
                required
                autoFocus
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="conn-password">Password</Label>
              <Input
                id="conn-password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="Enter password"
                required
              />
            </div>
          </>
        )}

        {configFields.map((field) => (
          <div key={field.key} className="flex flex-col gap-2">
            <Label htmlFor={`conn-${field.key}`}>{field.title}</Label>
            <Input
              id={`conn-${field.key}`}
              value={configValues[field.key] ?? ""}
              onChange={(event) => setConfigValue(field.key, event.target.value)}
              placeholder={field.placeholder}
              required
              autoFocus={!isApiKey && !isBasic && configFields[0]?.key === field.key}
            />
            {field.description && (
              <p className="text-xs text-muted-foreground">{field.description}</p>
            )}
          </div>
        ))}

        {isOutbound && (
          <p className="text-xs text-muted-foreground rounded-lg bg-muted p-3">
            After submitting, go to {integration.display_name}&apos;s settings to accept and install the integration.
          </p>
        )}

        <div className="mt-auto pt-2">
          <Button
            type="submit"
            className="w-full"
            disabled={!isValid() || isSubmitting}
          >
            {isSubmitting ? (
              <HugeiconsIcon icon={Loading03Icon} className="size-4 animate-spin" />
            ) : (
              "Connect"
            )}
          </Button>
        </div>
      </form>
    </div>
  )
}
