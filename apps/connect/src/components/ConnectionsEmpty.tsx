import { Button } from './Button'
import { PlusIcon } from './icons'

interface Props {
  onConnectNew: () => void
}

export function ConnectionsEmpty({ onConnectNew }: Props) {
  return (
    <div className="flex flex-col items-center justify-center grow gap-3 px-6">
      <div className="flex items-center justify-center rounded-xl bg-cw-surface shrink-0 size-16">
        <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
          <path d="M7 21V7a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v14" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
          <path d="M5 21h18" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
          <path d="M11 10h6M11 14h4" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
        </svg>
      </div>
      <div className="text-base text-cw-heading font-semibold leading-5">
        No providers connected
      </div>
      <div className="text-sm text-center text-cw-secondary leading-normal">
        Connect your first LLM provider to start using AI models in your application.
      </div>
      <Button onClick={onConnectNew} className="w-full mt-3 gap-2">
        <PlusIcon />
        Connect a provider
      </Button>
    </div>
  )
}
