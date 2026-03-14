import type { Connection } from '../types'
import { useProviders } from '../hooks/useProviders'
import { formatDate } from '../lib/utils'
import { ProviderLogo } from './ProviderLogo'

interface Props {
  connection: Connection
  isDark: boolean
  onClick: () => void
}

export function ConnectionCard({ connection, isDark, onClick }: Props) {
  const { data: providers = [] } = useProviders()
  const provider = providers.find((p) => p.id === connection.provider_id)

  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-3.5 bg-cw-surface border border-solid border-cw-border p-4 cursor-pointer hover:border-cw-placeholder transition-colors text-left w-full ${
        isDark ? 'rounded-xl' : 'rounded-2.5'
      }`}
    >
      <ProviderLogo providerId={connection.provider_id ?? ''} size="size-10" />
      <div className="flex flex-col grow shrink basis-0 gap-0.5">
        <div className="text-[15px] text-cw-heading font-semibold leading-4.5">
          {provider?.name ?? connection.provider_name ?? connection.provider_id}
        </div>
        <div className="text-xs text-cw-secondary leading-4">
          Connected {connection.created_at ? formatDate(connection.created_at) : ''}
        </div>
      </div>
      <div className="flex items-center gap-1.25">
        <div className="size-1.75 rounded-full bg-cw-success shrink-0" />
        <div className="text-[13px] text-cw-success font-medium leading-4">
          Active
        </div>
      </div>
    </button>
  )
}
