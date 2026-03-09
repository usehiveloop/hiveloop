import { providers, type ConnectedProvider } from '../data/providers'
import { useResolvedTheme } from '../hooks/ThemeContext'
import { Footer } from './Footer'

interface Props {
  connections: ConnectedProvider[]
  onViewDetail: (connection: ConnectedProvider) => void
  onConnectNew: () => void
  onClose: () => void
}

export function ConnectedList({ connections, onViewDetail, onConnectNew, onClose }: Props) {
  const theme = useResolvedTheme()
  const isDark = theme === 'dark'

  return (
    <div className="flex flex-col h-full pb-8">
      {/* Header */}
      <div className="flex items-center justify-between shrink-0">
        <div className="text-[20px] tracking-[-0.02em] text-cw-heading font-bold leading-6">
          Connected providers
        </div>
        <button onClick={onClose} className="cursor-pointer bg-transparent border-none p-0">
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
            <path d="M15 5L5 15M5 5l10 10" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      </div>

      {/* Provider cards */}
      <div className={`flex flex-col mt-6 ${isDark ? 'gap-3' : 'gap-2.5'}`}>
        {connections.map((conn) => {
          const provider = providers.find((p) => p.id === conn.providerId)!
          const isRevoked = conn.status === 'revoked'

          return (
            <button
              key={conn.id}
              onClick={() => onViewDetail(conn)}
              className={`flex items-center gap-3.5 bg-cw-surface border border-solid border-cw-border p-4 cursor-pointer hover:border-cw-placeholder transition-colors text-left w-full ${
                isDark ? 'rounded-[12px]' : 'rounded-[10px]'
              } ${isRevoked && isDark ? 'opacity-60' : ''}`}
            >
              <div className={`shrink-0 rounded-lg size-10 ${provider.colorClass}`} />
              <div className="flex flex-col grow shrink basis-[0%] gap-0.5">
                <div className="text-[15px] text-cw-heading font-semibold leading-[18px]">
                  {provider.name}
                </div>
                <div className="text-[12px] text-cw-secondary leading-4">
                  {conn.status === 'active' ? 'Connected' : 'Revoked'} {conn.connectedAt}
                </div>
              </div>
              {conn.status === 'active' ? (
                <div className="flex items-center gap-[5px]">
                  <div className="w-[7px] h-[7px] rounded-[50%] bg-cw-success shrink-0" />
                  <div className="text-[13px] text-cw-success font-medium leading-4">
                    Active
                  </div>
                </div>
              ) : (
                <div className="text-[13px] text-cw-error font-medium leading-4">
                  Revoked
                </div>
              )}
            </button>
          )
        })}
      </div>

      {/* Add button — Light: dashed border, accent text. Dark: solid accent bg, white text */}
      {isDark ? (
        <button
          onClick={onConnectNew}
          className="flex items-center justify-center mt-3 rounded-[12px] gap-2 bg-cw-accent p-3.5 cursor-pointer border-none hover:bg-cw-accent-hover transition-colors"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path d="M8 3v10M3 8h10" stroke="#FFFFFF" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
          <div className="text-[14px] text-white font-medium leading-[18px]">
            Connect new provider
          </div>
        </button>
      ) : (
        <button
          onClick={onConnectNew}
          className="flex items-center justify-center mt-1.5 rounded-[10px] gap-2 p-3.5 cursor-pointer bg-transparent border-[1.5px] border-dashed border-cw-border hover:bg-cw-surface transition-colors"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path d="M8 3v10M3 8h10" stroke="var(--color-cw-accent)" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
          <div className="text-[14px] text-cw-accent font-medium leading-[18px]">
            Connect new provider
          </div>
        </button>
      )}

      <Footer />
    </div>
  )
}
