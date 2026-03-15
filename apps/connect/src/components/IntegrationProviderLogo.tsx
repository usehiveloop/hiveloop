import { useState } from 'react'

const INTEGRATIONS_API = import.meta.env.VITE_INTEGRATIONS_API || 'https://integrations.dev.llmvault.dev'

interface Props {
  providerName: string
  className?: string
}

export function IntegrationProviderLogo({ providerName, className = 'size-10 rounded-lg' }: Props) {
  const [errored, setErrored] = useState(false)
  const src = `${INTEGRATIONS_API}/images/template-logos/${providerName}.svg`

  return (
    <div className={`shrink-0 flex items-center justify-center overflow-hidden bg-cw-surface ${className}`}>
      {errored ? (
        <span className="text-[11px] font-semibold uppercase text-cw-secondary">
          {providerName.slice(0, 2)}
        </span>
      ) : (
        <img
          src={src}
          alt=""
          className="h-3/5 w-3/5 object-contain"
          onError={() => setErrored(true)}
        />
      )}
    </div>
  )
}
