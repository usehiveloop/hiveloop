import { useEffect } from 'react'
import { providers } from '../data/providers'
import { Footer } from './Footer'

interface Props {
  providerId: string
  onSuccess: () => void
  onError: () => void
}

export function Validating({ providerId, onSuccess, onError }: Props) {
  const provider = providers.find((p) => p.id === providerId)!

  // Simulate validation — randomly succeed or fail after 2s
  useEffect(() => {
    const timer = setTimeout(() => {
      Math.random() > 0.3 ? onSuccess() : onError()
    }, 2000)
    return () => clearTimeout(timer)
  }, [onSuccess, onError])

  return (
    <div className="flex flex-col items-center justify-center h-full gap-4">
      {/* Spinner */}
      <div className="cw-spinner rounded-[50%] shrink-0 size-12 border-[3px] border-solid border-cw-border border-t-cw-accent" />
      <div className="text-[16px] text-cw-heading font-semibold leading-5">
        Connecting to {provider.name}...
      </div>
      <div className="text-[14px] text-cw-secondary leading-[18px]">
        Validating your API key
      </div>
      <Footer />
    </div>
  )
}
