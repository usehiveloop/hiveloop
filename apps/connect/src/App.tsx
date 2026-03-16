import { useState, useEffect, useMemo } from 'react'
import { AnimatePresence } from 'motion/react'
import { setOnUnauthorized } from './api/client'
import { useTheme } from './hooks/useTheme'
import { useThemeOverrides } from './hooks/useThemeOverrides'
import { useWidget } from './hooks/useWidget'
import { useSession } from './hooks/useSession'
import { useParentMessaging } from './hooks/useParentMessaging'
import { ThemeProvider } from './hooks/ThemeContext'
import { ConnectProvider } from './hooks/ConnectContext'
import { ParentEventProvider } from './hooks/ParentEventContext'
import { ViewRouter } from './components/ViewRouter'
import { AnimatedView, FadeView } from './components/AnimatedView'
import { Error } from './components/Error'
import { Loading } from './components/Loading'
import type { View } from './types'

function getInitialView(): View {
  const params = new URLSearchParams(window.location.search)
  const screen = params.get('screen')
  switch (screen) {
    case 'provider-selection':  return { type: 'provider-selection' }
    case 'api-key-input':       return { type: 'api-key-input', providerId: 'openai' }
    case 'validating':          return { type: 'validating', providerId: 'openai' }
    case 'success':             return { type: 'success', providerId: 'openai', connectionId: 'preview' }
    case 'error':               return { type: 'error', providerId: 'openai' }
    case 'connected-list':      return { type: 'connected-list' }
    case 'empty-state':         return { type: 'empty-state' }
    case 'integration-selection': return { type: 'integration-selection' }
    case 'integration-auth':      return { type: 'integration-auth', integration: { id: '', provider: 'slack', display_name: 'Slack', auth_mode: 'OAUTH2' } }
    case 'integration-success':   return { type: 'integration-success', integration: { id: '', provider: 'slack', display_name: 'Slack', auth_mode: 'OAUTH2' } }
    case 'integration-error':     return { type: 'integration-error', integration: { id: '', provider: 'slack', display_name: 'Slack', auth_mode: 'OAUTH2' }, error: 'Preview error' }
    case 'provider-connect':      return { type: 'provider-connect', providerId: params.get('providerId') ?? 'openai' }
    case 'integration-connect':   return { type: 'integration-connect', provider: params.get('integrationId') ?? '' }
    default:                    return { type: 'provider-selection' }
  }
}

function App() {
  useThemeOverrides()

  const params = new URLSearchParams(window.location.search)
  const { resolved } = useTheme(
    (params.get('theme') as 'light' | 'dark') || 'system'
  )
  const { view, direction, canGoBack, returnTo, navigate } = useWidget(getInitialView())
  const [sessionExpired, setSessionExpired] = useState(false)
  const { sendToParent, isEmbedded } = useParentMessaging()

  useEffect(() => {
    setOnUnauthorized(() => {
      setSessionExpired(true)
      sendToParent({
        type: 'error',
        payload: { code: 'session_invalid', message: 'This session has expired or is invalid.' },
      })
    })
    return () => setOnUnauthorized(null)
  }, [sendToParent])

  const connectState = useMemo(() => {
    const sessionId = params.get('session')
    const preview = params.get('preview') === 'true'
    return { sessionId, preview }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const needsSession = !connectState.preview
  const missingSession = needsSession && connectState.sessionId == null
  const { isLoading: sessionLoading, isError: sessionQueryError } = useSession(
    needsSession ? connectState.sessionId : null
  )

  const loading = needsSession && !missingSession && sessionLoading
  const sessionError = missingSession || sessionQueryError || sessionExpired

  const onClose = () => {
    sendToParent({ type: 'close' })
    navigate({ type: 'CANCEL' })
  }

  function renderContent() {
    if (loading) {
      return <FadeView viewKey="loading"><Loading /></FadeView>
    }

    if (sessionError) {
      return (
        <FadeView viewKey="session-error">
          <Error
            title="Session invalid"
            message="This session has expired or is invalid. Please close and try again."
            retryLabel="Close"
            onRetry={onClose}
            onCancel={onClose}
          />
        </FadeView>
      )
    }

    return (
      <AnimatedView viewKey={view.type} direction={direction}>
        <ViewRouter
          view={view}
          canGoBack={canGoBack}
          returnTo={returnTo}
          navigate={navigate}
          onClose={onClose}
        />
      </AnimatedView>
    )
  }

  return (
    <div className={`fixed inset-0 ${resolved === 'dark' ? 'dark' : ''}`}>
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative h-full w-full flex items-center justify-center pointer-events-none">
        <div className="connect-widget pointer-events-auto">
          <ConnectProvider sessionId={connectState.sessionId} preview={connectState.preview}>
          <ParentEventProvider sendToParent={sendToParent} isEmbedded={isEmbedded}>
          <ThemeProvider value={resolved}>
            <AnimatePresence custom={direction}>
              {renderContent()}
            </AnimatePresence>
          </ThemeProvider>
          </ParentEventProvider>
          </ConnectProvider>
        </div>
      </div>
    </div>
  )
}

export default App
