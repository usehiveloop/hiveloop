import type { IntegrationProvider, IntegrationResource } from '../types'
import { Button } from './Button'
import { Footer } from './Footer'
import { IntegrationProviderLogo } from './IntegrationProviderLogo'
import { ResourceIcon } from './ResourceIcon'
import { IconButton } from './IconButton'
import { BackIcon, CloseIcon, ChevronRightIcon } from './icons'

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
  onSelectResource?: (resource: IntegrationResource) => void
}

export function IntegrationDetail({ integration, onDisconnect, onBack, onClose, onSelectResource }: Props) {
  const name = integration.display_name || integration.provider || ''
  const resources = integration.resources ?? []

  const rows = [
    { label: 'Provider', value: integration.provider ?? '—' },
    { label: 'Auth Mode', value: integration.auth_mode ? formatAuthMode(integration.auth_mode) : '—' },
  ]

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center shrink-0 gap-3">
        <IconButton onClick={onBack}>
          <BackIcon />
        </IconButton>
        <IntegrationProviderLogo providerName={integration.provider ?? ''} className="size-9 rounded-lg" />
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

      <div className="flex flex-col grow overflow-y-auto -mx-4 px-4">
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

        {resources.length > 0 && (
          <div className="flex flex-col mt-8">
            <div className="text-xs tracking-wider mb-3 uppercase text-cw-secondary font-semibold leading-3.5">
              Granular permissions
            </div>
            <div className="flex flex-col gap-2">
              {resources.map((resource) => (
                <button
                  key={resource.type}
                  onClick={() => onSelectResource?.(resource)}
                  className="flex items-center gap-3 p-3 bg-cw-surface rounded-xl border border-solid border-cw-border hover:border-cw-placeholder transition-colors text-left cursor-pointer"
                >
                  <ResourceIcon iconName={resource.icon} size={20} className="size-10 shrink-0" />
                  <div className="flex flex-col grow gap-0.5">
                    <div className="text-sm text-cw-heading font-medium leading-4.5">
                      {resource.display_name || resource.type}
                    </div>
                    {resource.description && (
                      <div className="text-xs text-cw-secondary leading-4">
                        {resource.description}
                      </div>
                    )}
                  </div>
                  <ChevronRightIcon className="text-cw-secondary shrink-0" />
                </button>
              ))}
            </div>
          </div>
        )}

        <div className="grow min-h-8" />

        <div className="flex flex-col gap-4 pt-4 pb-8">
          <Button
            variant="danger"
            onClick={onDisconnect}
            className="w-full bg-cw-error-bg border border-solid border-cw-error-bg text-cw-error hover:bg-cw-error-bg hover:opacity-80"
          >
            Disconnect
          </Button>
          <Footer />
        </div>
      </div>
    </div>
  )
}
