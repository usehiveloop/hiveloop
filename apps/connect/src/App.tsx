import { useRef, useState, useEffect } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { useTheme } from './hooks/useTheme'
import { useWidget } from './hooks/useWidget'
import { ThemeProvider } from './hooks/ThemeContext'
import { mockConnectedProviders } from './data/providers'
import { ProviderSelection } from './components/ProviderSelection'
import { ApiKeyInput } from './components/ApiKeyInput'
import { Validating } from './components/Validating'
import { Success } from './components/Success'
import { Error } from './components/Error'
import { ConnectedList } from './components/ConnectedList'
import { ProviderDetail } from './components/ProviderDetail'
import { RevokeConfirm } from './components/RevokeConfirm'
import { EmptyState } from './components/EmptyState'
import { Loading } from './components/Loading'
import type { View } from './types'

function getInitialView(): View {
  const screen = new URLSearchParams(window.location.search).get('screen')
  switch (screen) {
    case 'provider-selection':
      return { type: 'provider-selection' }
    case 'api-key-input':
      return { type: 'api-key-input', providerId: 'openai' }
    case 'validating':
      return { type: 'validating', providerId: 'openai' }
    case 'success':
      return { type: 'success', providerId: 'openai' }
    case 'error':
      return { type: 'error', providerId: 'openai' }
    case 'connected-list':
      return { type: 'connected-list' }
    case 'provider-detail':
      return { type: 'provider-detail', connection: mockConnectedProviders[0] }
    case 'revoke-confirm':
      return { type: 'revoke-confirm', connection: mockConnectedProviders[0] }
    case 'empty-state':
      return { type: 'empty-state' }
    default:
      return { type: 'provider-selection' }
  }
}

const SLIDE_OFFSET = 60

function App() {
  const [loading, setLoading] = useState(true)
  const { resolved } = useTheme(
    (new URLSearchParams(window.location.search).get('theme') as 'light' | 'dark') || 'system'
  )
  const { view, direction, navigate } = useWidget(getInitialView())
  const directionRef = useRef(direction)
  directionRef.current = direction

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 2000)
    return () => clearTimeout(timer)
  }, [])

  const onClose = () => navigate({ type: 'CANCEL' })

  function renderView() {
    switch (view.type) {
      case 'provider-selection':
        return (
          <ProviderSelection
            onSelect={(providerId) => navigate({ type: 'SELECT_PROVIDER', providerId })}
            onClose={onClose}
          />
        )
      case 'api-key-input':
        return (
          <ApiKeyInput
            providerId={view.providerId}
            onSubmit={() => navigate({ type: 'SUBMIT_KEY' })}
            onBack={() => navigate({ type: 'BACK' })}
            onClose={onClose}
          />
        )
      case 'validating':
        return (
          <Validating
            providerId={view.providerId}
            onSuccess={() => navigate({ type: 'CONNECTION_SUCCESS' })}
            onError={() => navigate({ type: 'CONNECTION_ERROR' })}
          />
        )
      case 'success':
        return (
          <Success
            providerId={view.providerId}
            onDone={() => navigate({ type: 'DONE' })}
          />
        )
      case 'error':
        return (
          <Error
            onRetry={() => navigate({ type: 'RETRY' })}
            onCancel={onClose}
          />
        )
      case 'connected-list':
        return (
          <ConnectedList
            connections={mockConnectedProviders}
            onViewDetail={(connection) => navigate({ type: 'VIEW_DETAIL', connection })}
            onConnectNew={() => navigate({ type: 'CONNECT_NEW' })}
            onClose={onClose}
          />
        )
      case 'provider-detail':
        return (
          <ProviderDetail
            connection={view.connection}
            onRevoke={() => navigate({ type: 'REVOKE', connection: view.connection })}
            onBack={() => navigate({ type: 'BACK' })}
            onClose={onClose}
          />
        )
      case 'revoke-confirm':
        return (
          <RevokeConfirm
            connection={view.connection}
            onConfirm={() => navigate({ type: 'CONFIRM_REVOKE' })}
            onCancel={() => navigate({ type: 'BACK' })}
          />
        )
      case 'empty-state':
        return (
          <EmptyState
            onConnect={() => navigate({ type: 'CONNECT_NEW' })}
            onClose={onClose}
          />
        )
    }
  }

  return (
    <div className={`fixed inset-0 ${resolved === 'dark' ? 'dark' : ''}`}>
      {/* Backdrop — semi-transparent overlay, click to close */}
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />

      {/* Centered widget */}
      <div className="relative h-full w-full flex items-center justify-center pointer-events-none">
        <div className="connect-widget pointer-events-auto">
          <ThemeProvider value={resolved}>
            <AnimatePresence custom={direction}>
              {loading ? (
                <motion.div
                  key="loading"
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                  transition={{ duration: 0.2 }}
                  className="absolute inset-0 p-[inherit]"
                >
                  <Loading />
                </motion.div>
              ) : (
                <motion.div
                  key={view.type}
                  custom={directionRef.current}
                  initial="enter"
                  animate="center"
                  exit="exit"
                  variants={{
                    enter: (dir: string) => ({
                      x: dir === 'forward' ? SLIDE_OFFSET : -SLIDE_OFFSET,
                      opacity: 0,
                    }),
                    center: {
                      x: 0,
                      opacity: 1,
                    },
                    exit: (dir: string) => ({
                      x: dir === 'forward' ? -SLIDE_OFFSET : SLIDE_OFFSET,
                      opacity: 0,
                    }),
                  }}
                  transition={{ duration: 0.2, ease: [0.25, 0.1, 0.25, 1] }}
                  className="absolute inset-0 p-[inherit]"
                >
                  {renderView()}
                </motion.div>
              )}
            </AnimatePresence>
          </ThemeProvider>
        </div>
      </div>
    </div>
  )
}

export default App
