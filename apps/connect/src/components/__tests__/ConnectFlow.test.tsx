import { describe, it, expect, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '../../test/server'
import { mockProviders } from '../../test/handlers'
import { renderWithProviders } from '../../test/render'
import { ViewRouter } from '../ViewRouter'
import { useWidget } from '../../hooks/useWidget'

/**
 * Integration tests for the "Connect a provider" flow.
 *
 * Renders the real ViewRouter + useWidget state machine with MSW-mocked API.
 * Every test drives the UI the way a user would: click, type, read the screen.
 */
function ConnectFlowHarness({
  initialView = { type: 'provider-selection' } as import('../../types').View,
  onClose,
}: {
  initialView?: import('../../types').View
  onClose?: () => void
}) {
  const { view, canGoBack, returnTo, navigate } = useWidget(initialView)
  const handleClose = onClose ?? (() => navigate({ type: 'CANCEL' }))
  return <ViewRouter view={view} canGoBack={canGoBack} returnTo={returnTo} navigate={navigate} onClose={handleClose} />
}

async function waitForProviders() {
  await screen.findByText('All Providers')
}

async function clickProviderInList(user: ReturnType<typeof userEvent.setup>, name: string) {
  await waitForProviders()
  const allButtons = screen.getAllByRole('button')
  const providerButton = allButtons.find(
    (btn) => btn.textContent?.includes(name) && btn.textContent?.includes('model'),
  )
  if (!providerButton) throw new Error(`Provider button for "${name}" not found in list`)
  await user.click(providerButton)
}

function getConnectButton() {
  return screen.getByText('Connect').closest('button')!
}

// ---------------------------------------------------------------------------
// Provider selection screen
// ---------------------------------------------------------------------------

describe('Provider selection', () => {
  it('shows all providers with their model counts', async () => {
    renderWithProviders(<ConnectFlowHarness />)
    await waitForProviders()

    for (const p of mockProviders) {
      expect(screen.getAllByText(p.name).length).toBeGreaterThanOrEqual(1)
    }
    expect(screen.getByText('45 models')).toBeInTheDocument()
    expect(screen.getByText('12 models')).toBeInTheDocument()
    expect(screen.getByText('5 models')).toBeInTheDocument()
  })

  it('shows a loading spinner before providers arrive', () => {
    renderWithProviders(<ConnectFlowHarness />)
    expect(document.querySelector('.cw-spinner')).toBeInTheDocument()
  })

  it('filters the list when the user types in the search box', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ConnectFlowHarness />)
    await waitForProviders()

    const searchInput = screen.getByPlaceholderText('Search providers...')
    await user.type(searchInput, 'mis')

    expect(screen.getByText('Mistral')).toBeInTheDocument()
    expect(screen.queryByText('Cohere')).not.toBeInTheDocument()
    expect(screen.queryByText('Popular')).not.toBeInTheDocument()
  })

  it('search is case insensitive', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ConnectFlowHarness />)
    await waitForProviders()

    await user.type(screen.getByPlaceholderText('Search providers...'), 'COHERE')

    expect(screen.getByText('Cohere')).toBeInTheDocument()
    expect(screen.queryByText('Mistral')).not.toBeInTheDocument()
  })

  it('restores the full list when search is cleared', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ConnectFlowHarness />)
    await waitForProviders()

    const searchInput = screen.getByPlaceholderText('Search providers...')
    await user.type(searchInput, 'cohere')
    expect(screen.queryByText('Mistral')).not.toBeInTheDocument()

    await user.clear(searchInput)
    expect(screen.getByText('Mistral')).toBeInTheDocument()
    expect(screen.getByText('Cohere')).toBeInTheDocument()
  })

  it('shows an error screen when providers fail to load', async () => {
    server.use(
      http.get('https://api.dev.llmvault.dev/v1/widget/providers', () =>
        HttpResponse.json({ error: 'server error' }, { status: 500 }),
      ),
    )

    renderWithProviders(<ConnectFlowHarness />)

    await waitFor(() => {
      expect(screen.getByText('Unable to load providers')).toBeInTheDocument()
    })
    expect(screen.getByText(/couldn't reach the server/)).toBeInTheDocument()
  })

  it('recovers after clicking Retry on the error screen', async () => {
    const user = userEvent.setup()
    let callCount = 0

    server.use(
      http.get('https://api.dev.llmvault.dev/v1/widget/providers', () => {
        callCount++
        if (callCount === 1) {
          return HttpResponse.json({ error: 'server error' }, { status: 500 })
        }
        return HttpResponse.json(mockProviders)
      }),
    )

    renderWithProviders(<ConnectFlowHarness />)

    await waitFor(() => {
      expect(screen.getByText('Unable to load providers')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Retry'))

    await waitFor(() => {
      expect(screen.getByText('Mistral')).toBeInTheDocument()
    })
  })
})

// ---------------------------------------------------------------------------
// Selecting a provider → API key input
// ---------------------------------------------------------------------------

describe('Selecting a provider', () => {
  it('clicking a provider opens the API key form for that provider', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ConnectFlowHarness />)

    await clickProviderInList(user, 'Mistral')

    expect(await screen.findByPlaceholderText('Paste your Mistral API key')).toBeInTheDocument()
    expect(screen.getByText('API Key')).toBeInTheDocument()
  })

  it('clicking a popular chip opens the API key form', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ConnectFlowHarness />)
    await waitForProviders()

    const popularHeading = screen.getByText('Popular')
    const popularSection = popularHeading.closest('div')!.parentElement!
    const chip = within(popularSection).getAllByText('OpenAI')[0].closest('button')!
    await user.click(chip)

    expect(await screen.findByPlaceholderText('Paste your OpenAI API key')).toBeInTheDocument()
  })

  it('the back button on the API key form returns to provider selection', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ConnectFlowHarness />)

    await clickProviderInList(user, 'Cohere')
    await screen.findByPlaceholderText('Paste your Cohere API key')

    const header = screen.getByText('Cohere').parentElement!
    await user.click(header.querySelector('button')!)

    await waitFor(() => {
      expect(screen.getByText('Connect a provider')).toBeInTheDocument()
    })
  })
})

// ---------------------------------------------------------------------------
// API key input form behaviour
// ---------------------------------------------------------------------------

describe('API key input form', () => {
  async function navigateToKeyInput(user: ReturnType<typeof userEvent.setup>) {
    renderWithProviders(<ConnectFlowHarness />)
    await clickProviderInList(user, 'Mistral')
    await screen.findByPlaceholderText('Paste your Mistral API key')
  }

  it('the connect button is disabled until a key is entered', async () => {
    const user = userEvent.setup()
    await navigateToKeyInput(user)

    expect(getConnectButton()).toBeDisabled()

    await user.type(screen.getByPlaceholderText('Paste your Mistral API key'), 'sk-abc')

    expect(getConnectButton()).toBeEnabled()
  })

  it('whitespace-only input keeps the button disabled', async () => {
    const user = userEvent.setup()
    await navigateToKeyInput(user)

    await user.type(screen.getByPlaceholderText('Paste your Mistral API key'), '   ')

    expect(getConnectButton()).toBeDisabled()
  })

  it('toggles the API key between hidden and visible', async () => {
    const user = userEvent.setup()
    await navigateToKeyInput(user)

    const input = screen.getByPlaceholderText('Paste your Mistral API key')
    expect(input).toHaveAttribute('type', 'password')

    const toggleBtn = within(input.parentElement!).getByRole('button')
    await user.click(toggleBtn)
    expect(input).toHaveAttribute('type', 'text')

    await user.click(toggleBtn)
    expect(input).toHaveAttribute('type', 'password')
  })

  it('shows a documentation link when the provider has one', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ConnectFlowHarness />)

    // OpenAI has a doc URL in our mock data
    await clickProviderInList(user, 'OpenAI')

    await waitFor(() => {
      expect(screen.getByText('platform.openai.com/api-keys')).toBeInTheDocument()
    })
  })

  it('shows fallback text when the provider has no doc URL', async () => {
    const user = userEvent.setup()
    await navigateToKeyInput(user)

    expect(screen.getByText('Paste your Mistral API key above.')).toBeInTheDocument()
  })

  it('shows the encryption security callout', async () => {
    const user = userEvent.setup()
    await navigateToKeyInput(user)

    expect(screen.getByText(/encrypted end-to-end with AES-256/)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Submitting a connection
// ---------------------------------------------------------------------------

describe('Submitting a connection', () => {
  it('succeeds and shows the connected confirmation', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ConnectFlowHarness />)

    await clickProviderInList(user, 'Mistral')

    await user.type(
      await screen.findByPlaceholderText('Paste your Mistral API key'),
      'sk-live-abc123',
    )
    await user.click(getConnectButton())

    await waitFor(() => {
      expect(screen.getByText('Connected')).toBeInTheDocument()
    })
    expect(screen.getByText(/Mistral is ready to use/)).toBeInTheDocument()
  })

  it('clicking Done after success returns to provider selection', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ConnectFlowHarness />)

    await clickProviderInList(user, 'Mistral')
    await user.type(
      await screen.findByPlaceholderText('Paste your Mistral API key'),
      'sk-live-abc123',
    )
    await user.click(getConnectButton())

    await screen.findByText('Connected')
    await user.click(screen.getByText('Done'))

    await waitFor(() => {
      expect(screen.getByText('Connect a provider')).toBeInTheDocument()
    })
  })

  it('sends provider_id, api_key, and label to the API', async () => {
    const user = userEvent.setup()
    let capturedBody: Record<string, unknown> | null = null

    server.use(
      http.post('https://api.dev.llmvault.dev/v1/widget/connections', async ({ request }) => {
        capturedBody = await request.json() as Record<string, unknown>
        return HttpResponse.json(
          {
            id: 'conn-new',
            label: capturedBody.label,
            provider_id: capturedBody.provider_id,
            provider_name: 'Mistral',
            base_url: 'https://api.mistral.ai',
            auth_scheme: 'bearer',
            created_at: new Date().toISOString(),
          },
          { status: 201 },
        )
      }),
    )

    renderWithProviders(<ConnectFlowHarness />)
    await clickProviderInList(user, 'Mistral')

    await user.type(
      await screen.findByPlaceholderText('Paste your Mistral API key'),
      'sk-mistral-key',
    )
    await user.type(screen.getByPlaceholderText('e.g. Production key'), 'Staging')
    await user.click(getConnectButton())

    await waitFor(() => {
      expect(capturedBody).toMatchObject({
        provider_id: 'mistral',
        api_key: 'sk-mistral-key',
        label: 'Staging',
      })
    })
  })

  it('omits label from the request when the field is left empty', async () => {
    const user = userEvent.setup()
    let capturedBody: Record<string, unknown> | null = null

    server.use(
      http.post('https://api.dev.llmvault.dev/v1/widget/connections', async ({ request }) => {
        capturedBody = await request.json() as Record<string, unknown>
        return HttpResponse.json(
          {
            id: 'conn-new',
            provider_id: 'mistral',
            provider_name: 'Mistral',
            base_url: 'https://api.mistral.ai',
            auth_scheme: 'bearer',
            created_at: new Date().toISOString(),
          },
          { status: 201 },
        )
      }),
    )

    renderWithProviders(<ConnectFlowHarness />)
    await clickProviderInList(user, 'Mistral')

    await user.type(
      await screen.findByPlaceholderText('Paste your Mistral API key'),
      'sk-test',
    )
    await user.click(getConnectButton())

    await waitFor(() => {
      expect(capturedBody).toBeDefined()
      expect(capturedBody!.label).toBeUndefined()
    })
  })
})

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

describe('Connection errors', () => {
  it('shows the error screen when the API rejects the key', async () => {
    const user = userEvent.setup()

    server.use(
      http.post('https://api.dev.llmvault.dev/v1/widget/connections', () =>
        HttpResponse.json({ error: 'invalid key' }, { status: 401 }),
      ),
    )

    renderWithProviders(<ConnectFlowHarness />)
    await clickProviderInList(user, 'Cohere')

    await user.type(
      await screen.findByPlaceholderText('Paste your Cohere API key'),
      'sk-bad-key',
    )
    await user.click(getConnectButton())

    await waitFor(() => {
      expect(screen.getByText('Connection failed')).toBeInTheDocument()
    })
    expect(screen.getByText(/could not be validated/)).toBeInTheDocument()
  })

  it('"Try again" returns to the API key form for the same provider', async () => {
    const user = userEvent.setup()

    server.use(
      http.post('https://api.dev.llmvault.dev/v1/widget/connections', () =>
        HttpResponse.json({ error: 'invalid key' }, { status: 401 }),
      ),
    )

    renderWithProviders(<ConnectFlowHarness />)
    await clickProviderInList(user, 'Cohere')

    await user.type(
      await screen.findByPlaceholderText('Paste your Cohere API key'),
      'sk-bad',
    )
    await user.click(getConnectButton())

    await screen.findByText('Connection failed')
    server.resetHandlers()

    await user.click(screen.getByText('Try again'))

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Paste your Cohere API key')).toBeInTheDocument()
    })
  })

  it('"Cancel" on the error screen closes the widget', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()

    server.use(
      http.post('https://api.dev.llmvault.dev/v1/widget/connections', () =>
        HttpResponse.json({ error: 'fail' }, { status: 500 }),
      ),
    )

    renderWithProviders(<ConnectFlowHarness onClose={onClose} />)
    await clickProviderInList(user, 'Mistral')

    await user.type(
      await screen.findByPlaceholderText('Paste your Mistral API key'),
      'sk-bad',
    )
    await user.click(getConnectButton())

    await screen.findByText('Connection failed')
    await user.click(screen.getByText('Cancel'))

    expect(onClose).toHaveBeenCalled()
  })
})

// ---------------------------------------------------------------------------
// Connected list → connect flow (returnTo navigation)
// ---------------------------------------------------------------------------

describe('Navigation from connected list', () => {
  it('shows existing connections on the connected list screen', async () => {
    renderWithProviders(
      <ConnectFlowHarness initialView={{ type: 'connected-list' }} />,
    )

    expect(await screen.findByText('Connected providers')).toBeInTheDocument()
    expect(await screen.findByText('Active')).toBeInTheDocument()
  })

  it('shows the empty state when there are no connections', async () => {
    server.use(
      http.get('https://api.dev.llmvault.dev/v1/widget/connections', () =>
        HttpResponse.json({ data: [], has_more: false }),
      ),
    )

    renderWithProviders(
      <ConnectFlowHarness initialView={{ type: 'connected-list' }} />,
    )

    expect(await screen.findByText('No providers connected')).toBeInTheDocument()
  })

  it('shows a back button on provider selection when coming from connected list', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <ConnectFlowHarness initialView={{ type: 'connected-list' }} />,
    )

    await user.click(await screen.findByText('Connect new provider'))

    await waitFor(() => {
      expect(screen.getByText('Connect a provider')).toBeInTheDocument()
    })

    // Back button should exist in the header (two buttons: back + close)
    const header = screen.getByText('Connect a provider').parentElement!
    const buttons = header.querySelectorAll('button')
    expect(buttons.length).toBe(2)
  })

  it('back button on provider selection returns to connected list', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <ConnectFlowHarness initialView={{ type: 'connected-list' }} />,
    )

    await user.click(await screen.findByText('Connect new provider'))
    await waitFor(() => {
      expect(screen.getByText('Connect a provider')).toBeInTheDocument()
    })

    // Click the back button (first button in header)
    const header = screen.getByText('Connect a provider').parentElement!
    await user.click(header.querySelector('button')!)

    expect(await screen.findByText('Connected providers')).toBeInTheDocument()
  })

  it('after successful connection, Done returns to connected list', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <ConnectFlowHarness initialView={{ type: 'connected-list' }} />,
    )

    // Navigate: connected list → connect new → select provider → enter key → success → done
    await user.click(await screen.findByText('Connect new provider'))
    await waitForProviders()
    await clickProviderInList(user, 'Mistral')

    await user.type(
      await screen.findByPlaceholderText('Paste your Mistral API key'),
      'sk-live-abc123',
    )
    await user.click(getConnectButton())

    await screen.findByText('Connected')
    await user.click(screen.getByText('Done'))

    expect(await screen.findByText('Connected providers')).toBeInTheDocument()
  })

  it('after connection error, Cancel returns to connected list', async () => {
    const user = userEvent.setup()

    server.use(
      http.post('https://api.dev.llmvault.dev/v1/widget/connections', () =>
        HttpResponse.json({ error: 'fail' }, { status: 500 }),
      ),
    )

    renderWithProviders(
      <ConnectFlowHarness initialView={{ type: 'connected-list' }} />,
    )

    await user.click(await screen.findByText('Connect new provider'))
    await waitForProviders()
    await clickProviderInList(user, 'Mistral')

    await user.type(
      await screen.findByPlaceholderText('Paste your Mistral API key'),
      'sk-bad',
    )
    await user.click(getConnectButton())

    await screen.findByText('Connection failed')
    await user.click(screen.getByText('Cancel'))

    expect(await screen.findByText('Connected providers')).toBeInTheDocument()
  })

  it('no back button on provider selection when it is the initial view', () => {
    renderWithProviders(<ConnectFlowHarness />)

    // Only the close button should exist in the header
    const header = screen.getByText('Connect a provider').parentElement!
    const buttons = header.querySelectorAll('button')
    expect(buttons.length).toBe(1)
  })

  it('revoke flow returns to connected list after success', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <ConnectFlowHarness initialView={{ type: 'connected-list' }} />,
    )

    // Click a connection card to view detail
    await user.click(await screen.findByText('Active'))

    // Click revoke
    await user.click(await screen.findByText('Revoke access'))

    // Confirm revoke
    await user.click(await screen.findByText('Yes, revoke access'))

    // Should show revoke success
    expect(await screen.findByText('Access revoked')).toBeInTheDocument()

    // Click back to providers
    await user.click(screen.getByText('Back to providers'))

    expect(await screen.findByText('Connected providers')).toBeInTheDocument()
  })
})
