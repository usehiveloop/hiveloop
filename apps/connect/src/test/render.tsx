import { type ReactElement } from 'react'
import { render, type RenderOptions } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConnectProvider } from '../hooks/ConnectContext'
import { ThemeProvider } from '../hooks/ThemeContext'

interface WrapperOptions {
  sessionId?: string | null
  preview?: boolean
  theme?: 'light' | 'dark'
}

function createWrapper({
  sessionId = 'test-session-token',
  preview = false,
  theme = 'light',
}: WrapperOptions = {}) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })

  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <ConnectProvider sessionId={sessionId} preview={preview}>
          <ThemeProvider value={theme}>{children}</ThemeProvider>
        </ConnectProvider>
      </QueryClientProvider>
    )
  }
}

export function renderWithProviders(
  ui: ReactElement,
  options?: WrapperOptions & Omit<RenderOptions, 'wrapper'>,
) {
  const { sessionId, preview, theme, ...renderOptions } = options ?? {}
  return render(ui, {
    wrapper: createWrapper({ sessionId, preview, theme }),
    ...renderOptions,
  })
}
