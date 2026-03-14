import type { Connection } from '../types'
import { useProviders } from '../hooks/useProviders'
import { formatDate } from '../lib/utils'
import { Button } from './Button'
import { Footer } from './Footer'
import { ProviderLogo } from './ProviderLogo'
import { IconButton } from './IconButton'
import { BackIcon, CloseIcon } from './icons'

interface Props {
  connection: Connection
  onRevoke: () => void
  onBack: () => void
  onClose: () => void
}

export function ProviderDetail({ connection, onRevoke, onBack, onClose }: Props) {
  const { data: providers = [] } = useProviders()
  const provider = providers.find((p) => p.id === connection.provider_id)
  const providerName = provider?.name ?? connection.provider_name ?? connection.provider_id ?? ''

  const rows = [
    { label: 'Provider', value: providerName },
    { label: 'Connected', value: connection.created_at ? formatDate(connection.created_at) : '—' },
    { label: 'Label', value: connection.label || '—' },
    { label: 'Auth Scheme', value: connection.auth_scheme ?? '—' },
  ]

  return (
    <div className="flex flex-col h-full pb-8">
      {/* Header */}
      <div className="flex items-center shrink-0 gap-3">
        <IconButton onClick={onBack}>
          <BackIcon />
        </IconButton>
        <ProviderLogo providerId={connection.provider_id ?? ''} size="size-9" />
        <div className="flex flex-col grow shrink basis-0 gap-px">
          <div className="text-lg text-cw-heading font-bold leading-5.5">{providerName}</div>
          <div className="flex items-center gap-1.25">
            <div className="rounded-full bg-cw-success shrink-0 size-1.5" />
            <div className="text-xs text-cw-success leading-4">Active</div>
          </div>
        </div>
        <IconButton onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </div>

      {/* Detail rows */}
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

      {/* Revoke button */}
      <Button
        variant="danger"
        onClick={onRevoke}
        className="mt-6 bg-cw-error-bg border border-solid border-cw-error-bg text-cw-error hover:bg-cw-error-bg hover:opacity-80"
      >
        Revoke access
      </Button>

      <Footer />
    </div>
  )
}
