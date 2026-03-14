import type { Connection } from '../types'
import { useResolvedTheme } from '../hooks/useResolvedTheme'
import { useConnections } from '../hooks/useConnections'
import { Button } from './Button'
import { ConnectionsEmpty } from './ConnectionsEmpty'
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
        <ConnectionsEmpty onConnectNew={onConnectNew} />
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
