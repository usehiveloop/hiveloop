import { providers } from '../data/providers'
import { Footer } from './Footer'

interface Props {
  providerId: string
  onDone: () => void
}

export function Success({ providerId, onDone }: Props) {
  const provider = providers.find((p) => p.id === providerId)!

  return (
    <div className="flex flex-col items-center justify-center h-full py-7 px-12 gap-3">
      {/* Green check circle */}
      <div className="flex items-center justify-center rounded-[50%] bg-cw-success-bg shrink-0 size-14">
        <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
          <path d="M8 14.5l4 4 8-8" stroke="var(--color-cw-success)" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </div>
      <div className="text-[20px] text-cw-heading font-bold leading-6">
        Connected
      </div>
      <div className="text-[14px] text-center leading-[150%] text-cw-secondary">
        {provider.name} is ready to use. Your API key has been encrypted and stored securely.
      </div>
      <button
        onClick={onDone}
        className="flex items-center justify-center w-full mt-3 rounded-lg bg-cw-accent p-3.5 cursor-pointer border-none hover:bg-cw-accent-hover active:bg-cw-accent-active transition-colors"
      >
        <div className="text-[15px] text-white font-semibold leading-[18px]">
          Done
        </div>
      </button>
      <Footer />
    </div>
  )
}
