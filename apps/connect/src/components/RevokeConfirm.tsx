import { providers, type ConnectedProvider } from '../data/providers'
import { Footer } from './Footer'

interface Props {
  connection: ConnectedProvider
  onConfirm: () => void
  onCancel: () => void
}

export function RevokeConfirm({ connection, onConfirm, onCancel }: Props) {
  const provider = providers.find((p) => p.id === connection.providerId)!

  return (
    <div className="flex flex-col items-center justify-center h-full py-7 px-12 gap-3">
      {/* Red warning circle */}
      <div className="flex items-center justify-center rounded-[50%] bg-cw-error-bg shrink-0 size-14">
        <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
          <path d="M14 9v6M14 19h.01" stroke="var(--color-cw-error)" strokeWidth="2.5" strokeLinecap="round" />
        </svg>
      </div>
      <div className="text-[20px] text-cw-heading font-bold leading-6">
        Revoke {provider.name}?
      </div>
      <div className="text-[14px] text-center leading-[150%] text-cw-secondary">
        This will permanently revoke the API key. Any apps using this connection will stop working.
      </div>
      <div className="flex flex-col w-full mt-3 gap-2.5">
        <button
          onClick={onConfirm}
          className="flex items-center justify-center rounded-lg bg-cw-error p-3.5 cursor-pointer border-none hover:bg-cw-error-hover transition-colors"
        >
          <div className="text-[15px] text-white font-semibold leading-[18px]">
            Yes, revoke access
          </div>
        </button>
        <button
          onClick={onCancel}
          className="flex items-center justify-center rounded-lg bg-cw-surface border border-solid border-cw-border p-3.5 cursor-pointer hover:bg-cw-divider transition-colors"
        >
          <div className="text-[15px] text-cw-body font-medium leading-[18px]">
            Cancel
          </div>
        </button>
      </div>
      <Footer />
    </div>
  )
}
