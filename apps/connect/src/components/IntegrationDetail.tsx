import type { IntegrationProvider } from '../types'
import { Button } from './Button'
import { Footer } from './Footer'
import { IntegrationProviderLogo } from './IntegrationProviderLogo'
import { IconButton } from './IconButton'
import { BackIcon, CloseIcon } from './icons'

function formatAuthMode(mode: string): string {
  switch (mode) {
    case 'OAUTH2': return 'OAuth 2.0'
    case 'OAUTH1': return 'OAuth 1.0'
    case 'OAUTH2_CC': return 'OAuth 2.0 (Client Credentials)'
    case 'API_KEY': return 'API Key'
    case 'BASIC': return 'Basic Auth'
    case 'APP_STORE': return 'App Store'
    case 'CUSTOM': return 'Custom'
    case 'TBA': return 'Token-Based Auth'
    case 'TABLEAU': return 'Tableau'
    case 'JWT': return 'JWT'
    case 'BILL': return 'Bill'
    case 'TWO_STEP': return 'Two-Step'
    case 'SIGNATURE': return 'Signature'
    default: return mode
  }
}

interface Props {
  integration: IntegrationProvider
  onDisconnect: () => void
  onBack: () => void
  onClose: () => void
}

export function IntegrationDetail({ integration, onDisconnect, onBack, onClose }: Props) {
  const name = integration.display_name || integration.provider || ''

  const rows = [
    { label: 'Provider', value: integration.provider ?? '—' },
    { label: 'Auth Mode', value: integration.auth_mode ? formatAuthMode(integration.auth_mode) : '—' },
  ]

  return (
    <div className="flex flex-col h-full pb-8">
      <div className="flex items-center shrink-0 gap-3">
        <IconButton onClick={onBack}>
          <BackIcon />
        </IconButton>
        <IntegrationProviderLogo providerName={integration.provider ?? ''} size="size-9" />
        <div className="flex flex-col grow shrink basis-0 gap-px">
          <div className="text-lg text-cw-heading font-bold leading-5.5">{name}</div>
          <div className="flex items-center gap-1.25">
            <div className="rounded-full bg-cw-success shrink-0 size-1.5" />
            <div className="text-xs text-cw-success leading-4">Connected</div>
          </div>
        </div>
        <IconButton onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </div>

      <div className="flex flex-col mt-7">
        {rows.map((row, i) => (
          <div
            key={row.label}
            className={`flex justify-between py-3.5 ${
              i < rows.length - 1 ? 'border-b border-b-solid border-b-cw-divider' : ''
            }`}
          >
            <div className="text-[13px] text-cw-secondary leading-4">{row.label}</div>
            <div className="text-[13px] text-cw-heading font-medium leading-4">{row.value}</div>
          </div>
        ))}
      </div>

      <Button
        variant="danger"
        onClick={onDisconnect}
        className="mt-6 bg-cw-error-bg border border-solid border-cw-error-bg text-cw-error hover:bg-cw-error-bg hover:opacity-80"
      >
        Disconnect
      </Button>

      <Footer />
    </div>
  )
}
