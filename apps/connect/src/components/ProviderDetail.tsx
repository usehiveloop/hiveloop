import { providers, type ConnectedProvider } from '../data/providers'
import { Footer } from './Footer'

interface Props {
  connection: ConnectedProvider
  onRevoke: () => void
  onBack: () => void
  onClose: () => void
}

export function ProviderDetail({ connection, onRevoke, onBack, onClose }: Props) {
  const provider = providers.find((p) => p.id === connection.providerId)!

  const rows = [
    { label: 'Provider', value: provider.name },
    { label: 'Connected', value: connection.connectedAt },
    { label: 'Label', value: connection.label },
    { label: 'API Key', value: connection.maskedKey },
    { label: 'Requests', value: connection.requests.toLocaleString() },
  ]

  return (
    <div className="flex flex-col h-full pb-8">
      {/* Header */}
      <div className="flex items-center shrink-0 gap-3">
        <button onClick={onBack} className="cursor-pointer bg-transparent border-none p-0">
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
            <path d="M13 4L7 10l6 6" stroke="var(--color-cw-icon-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </button>
        <div className={`shrink-0 rounded-lg size-9 ${provider.colorClass}`} />
        <div className="flex flex-col grow shrink basis-[0%] gap-px">
          <div className="text-[18px] text-cw-heading font-bold leading-[22px]">
            {provider.name}
          </div>
          <div className="flex items-center gap-[5px]">
            {connection.status === 'active' ? (
              <>
                <div className="rounded-[50%] bg-cw-success shrink-0 size-1.5" />
                <div className="text-[12px] text-cw-success leading-4">
                  Active
                </div>
              </>
            ) : (
              <div className="text-[12px] text-cw-error leading-4">
                Revoked
              </div>
            )}
          </div>
        </div>
        <button onClick={onClose} className="cursor-pointer bg-transparent border-none p-0">
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
            <path d="M15 5L5 15M5 5l10 10" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
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
            <div className="text-[13px] text-cw-secondary leading-4">
              {row.label}
            </div>
            <div className="text-[13px] text-cw-heading font-medium leading-4">
              {row.value}
            </div>
          </div>
        ))}
      </div>

      {/* Revoke button */}
      {connection.status === 'active' && (
        <button
          onClick={onRevoke}
          className="flex items-center justify-center mt-6 rounded-lg bg-cw-error-bg border border-solid border-cw-error-bg p-3.5 cursor-pointer hover:opacity-80 transition-colors"
        >
          <div className="text-[15px] text-cw-error font-semibold leading-[18px]">
            Revoke access
          </div>
        </button>
      )}

      <Footer />
    </div>
  )
}
