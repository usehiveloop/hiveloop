import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useProviders } from '../hooks/useProviders'
import { useConnect } from '../hooks/useConnect'
import { createWidgetFetchClient } from '../api/client'
import { Button } from './Button'
import { Footer } from './Footer'
import { ProviderLogo } from './ProviderLogo'
import { SecurityCallout } from './SecurityCallout'
import { IconButton } from './IconButton'
import { BackIcon, CloseIcon, EyeIcon, EyeOffIcon } from './icons'

interface Props {
  providerId: string
  onSubmit: () => void
  onSuccess: (connectionId: string) => void
  onError: () => void
  onBack: () => void
  onClose: () => void
}

export function ApiKeyInput({ providerId, onSubmit, onSuccess, onError, onBack, onClose }: Props) {
  const queryClient = useQueryClient()
  const { data: providers = [] } = useProviders()
  const { pending, setPending, sessionId, preview } = useConnect()
  const provider = providers.find((p) => p.id === providerId)
  const [apiKey, setApiKey] = useState(pending?.providerId === providerId ? pending.apiKey : '')
  const [label, setLabel] = useState(pending?.providerId === providerId ? pending.label : '')
  const [showKey, setShowKey] = useState(false)

  const mutation = useMutation({
    mutationFn: async (body: { provider_id: string; api_key: string; label?: string }) => {
      if (preview) {
        await new Promise((r) => setTimeout(r, 1500))
        return { id: 'preview' } as { id: string }
      }
      const client = createWidgetFetchClient(sessionId!)
      const { data } = await client.POST('/v1/widget/connections', { body })
      return data as { id: string }
    },
    onSuccess: (data) => {
      setPending(null)
      queryClient.invalidateQueries({ queryKey: ['widget', 'connections'] })
      onSuccess(data?.id ?? '')
    },
    onError: () => onError(),
  })

  function handleConnect() {
    setPending({ providerId, apiKey, label })
    onSubmit()
    mutation.mutate({
      provider_id: providerId,
      api_key: apiKey,
      label: label || undefined,
    })
  }

  const providerName = provider?.name ?? providerId

  return (
    <div className="flex flex-col h-full pb-8">
      {/* Header */}
      <div className="flex items-center shrink-0 gap-3.5">
        <IconButton onClick={onBack}>
          <BackIcon />
        </IconButton>
        <ProviderLogo providerId={providerId} size="size-8" />
        <div className="text-lg grow shrink basis-0 text-cw-heading cw-mobile:font-semibold cw-desktop:font-bold leading-5.5">
          {providerName}
        </div>
        <IconButton onClick={onClose} className="cw-mobile:hidden">
          <CloseIcon />
        </IconButton>
      </div>

      {/* Form */}
      <div className="flex flex-col cw-mobile:mt-8 cw-desktop:mt-7 shrink-0 cw-mobile:gap-5 cw-desktop:gap-4">
        {/* API Key field */}
        <div className="flex flex-col gap-1.5">
          <div className="text-[13px] text-cw-heading cw-mobile:font-medium cw-desktop:font-semibold leading-4">
            API Key
          </div>
          <div className="flex items-center cw-mobile:rounded-2.5 cw-desktop:rounded-lg py-3 px-3.5 gap-2 cw-mobile:bg-cw-bg cw-mobile:border cw-mobile:border-solid cw-mobile:border-cw-border cw-desktop:bg-cw-surface cw-desktop:border cw-desktop:border-solid cw-desktop:border-cw-border focus-within:border-cw-accent focus-within:ring-2 focus-within:ring-cw-accent-subtle transition-colors">
            <input
              type={showKey ? 'text' : 'password'}
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={`Paste your ${providerName} API key`}
              className="text-sm grow shrink basis-0 bg-transparent border-none outline-none text-cw-heading leading-4.5 placeholder:text-cw-input-placeholder"
            />
            <IconButton onClick={() => setShowKey(!showKey)} className="shrink-0">
              {showKey ? <EyeOffIcon /> : <EyeIcon />}
            </IconButton>
          </div>
          {provider?.doc ? (
            <div className="cw-desktop:block cw-mobile:hidden">
              <span className="text-xs text-cw-secondary leading-4">Find your API key at </span>
              <a href={provider.doc} target="_blank" rel="noopener noreferrer" className="text-xs text-cw-accent leading-4 no-underline hover:underline">
                {provider.doc.replace(/^https?:\/\//, '')}
              </a>
            </div>
          ) : (
            <div className="cw-desktop:block cw-mobile:hidden">
              <span className="text-xs text-cw-secondary leading-4">Paste your {providerName} API key above.</span>
            </div>
          )}
          <div className="cw-mobile:block cw-desktop:hidden text-xs text-cw-secondary leading-4">
            Paste your {providerName} API key. It will be encrypted before storage.
          </div>
        </div>

        {/* Label field */}
        <div className="flex flex-col gap-1.5">
          <div className="cw-mobile:hidden flex items-baseline gap-1.5">
            <div className="text-[13px] text-cw-heading font-semibold leading-4">Label</div>
            <div className="text-xs text-cw-input-placeholder leading-4">— optional</div>
          </div>
          <div className="cw-desktop:hidden text-[13px] text-cw-heading font-medium leading-4">
            Label (optional)
          </div>
          <div className="flex items-center cw-mobile:rounded-2.5 cw-desktop:rounded-lg py-3 px-3.5 cw-mobile:bg-cw-bg cw-mobile:border cw-mobile:border-solid cw-mobile:border-cw-border cw-desktop:bg-cw-surface cw-desktop:border cw-desktop:border-solid cw-desktop:border-cw-border focus-within:border-cw-accent focus-within:ring-2 focus-within:ring-cw-accent-subtle transition-colors">
            <input
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="e.g. Production key"
              className="text-sm bg-transparent border-none outline-none text-cw-heading leading-4.5 w-full placeholder:text-cw-input-placeholder"
            />
          </div>
        </div>
      </div>

      <SecurityCallout />

      {/* Connect button */}
      <Button
        onClick={handleConnect}
        disabled={!apiKey.trim()}
        loading={mutation.isPending}
        className="cw-mobile:mt-6 cw-desktop:mt-4 shrink-0 cw-mobile:rounded-2.5"
      >
        <span className="cw-desktop:hidden">Connect {providerName}</span>
        <span className="cw-mobile:hidden">Connect</span>
      </Button>

      <Footer />
    </div>
  )
}
