import { useProviders } from '../hooks/useProviders'
import { Footer } from './Footer'

interface Props {
  providerId: string
}

export function Validating({ providerId }: Props) {
  const { data: providers = [] } = useProviders()
  const provider = providers.find((p) => p.id === providerId)

  return (
    <div className="flex flex-col items-center justify-center h-full gap-4">
      {/* Spinner */}
      <div className="cw-spinner rounded-full shrink-0 size-12 border-3 border-solid border-cw-border border-t-cw-accent" />
      <div className="text-base text-cw-heading font-semibold leading-5">
        Connecting to {provider?.name ?? providerId}...
      </div>
      <div className="text-sm text-cw-secondary leading-4.5">
        Validating your API key
      </div>
      <Footer />
    </div>
  )
}
