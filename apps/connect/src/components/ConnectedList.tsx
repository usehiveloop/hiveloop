import type { Connection } from '../types'
import { useResolvedTheme } from '../hooks/useResolvedTheme'
import { useConnections } from '../hooks/useConnections'
import { Button } from './Button'
import { Loading } from './Loading'
import { Footer } from './Footer'
import { PageHeader } from './PageHeader'
import { ConnectionCard } from './ConnectionCard'
import { PlusIcon } from './icons'

interface Props {
  onViewDetail: (connection: Connection) => void
  onConnectNew: () => void
  onClose: () => void
}

export function ConnectedList({ onViewDetail, onConnectNew, onClose }: Props) {
  const { data: connections = [], isLoading } = useConnections()
  const theme = useResolvedTheme()
  const isDark = theme === 'dark'

  if (isLoading) return <Loading />

  return (
    <div className="flex flex-col h-full pb-8">
      <PageHeader title="Connected providers" onClose={onClose} />

      {connections.length === 0 ? (
        <div className="flex flex-col items-center justify-center grow gap-3 px-6">
          <div className="flex items-center justify-center rounded-full bg-cw-surface shrink-0 size-14">
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
      ) : (
        <>
          {/* Connection cards */}
          <div className={`flex flex-col mt-6 min-h-0 overflow-y-auto ${isDark ? 'gap-3' : 'gap-2.5'}`}>
            {connections.map((conn) => (
              <ConnectionCard
                key={conn.id}
                connection={conn}
                isDark={isDark}
                onClick={() => onViewDetail(conn)}
              />
            ))}
          </div>

          {/* Add button */}
          <Button onClick={onConnectNew} className="mt-auto shrink-0 gap-2">
            <PlusIcon />
            Connect new provider
          </Button>
        </>
      )}

      <Footer />
    </div>
  )
}
