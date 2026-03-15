import { useState, useMemo, useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { popularIntegrationNames } from '../data/integrations'
import { useIntegrationProviders } from '../hooks/useIntegrationProviders'
import type { IntegrationProvider } from '../types'
import { Error } from './Error'
import { Footer } from './Footer'
import { IntegrationProviderLogo } from './IntegrationProviderLogo'
import { IconButton } from './IconButton'
import { BackIcon, CloseIcon, SearchIcon, ChevronRightIcon, SpinnerIcon, CheckIcon } from './icons'

function formatAuthMode(mode: string): string {
  switch (mode) {
    case 'OAUTH2': return 'OAuth 2.0'
    case 'OAUTH1': return 'OAuth 1.0'
    case 'OAUTH2_CC': return 'OAuth 2.0 (Client Credentials)'
    case 'API_KEY': return 'API Key'
    case 'BASIC': return 'Basic Auth'
    case 'APP_STORE': return 'App Store'
    case 'CUSTOM': return 'Custom'
    case 'TBA': return 'Token-Based Auth'
    case 'TABLEAU': return 'Tableau'
    case 'JWT': return 'JWT'
    case 'BILL': return 'Bill'
    case 'TWO_STEP': return 'Two-Step'
    case 'SIGNATURE': return 'Signature'
    default: return mode
  }
}

interface Props {
  onSelect: (integration: IntegrationProvider) => void
  onViewDetail: (integration: IntegrationProvider) => void
  onBack?: () => void
  onClose: () => void
}

export function IntegrationProviderSelection({ onSelect, onViewDetail, onBack, onClose }: Props) {
  const [search, setSearch] = useState('')
  const { data: providers = [], isLoading, isError, refetch } = useIntegrationProviders()

  const popular = useMemo(
    () => providers.filter((p) => popularIntegrationNames.includes(p.provider ?? '')),
    [providers]
  )

  const filtered = useMemo(
    () =>
      search.trim()
        ? providers.filter((p) => {
            const q = search.toLowerCase()
            return (p.provider ?? '').toLowerCase().includes(q) || (p.display_name ?? '').toLowerCase().includes(q)
          })
        : providers,
    [search, providers]
  )

  const scrollRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 52,
    overscan: 20,
  })

  if (isError) {
    return (
      <Error
        title="Unable to load integrations"
        message="We couldn't reach the server to load available integrations. Please check your connection and try again."
        retryLabel="Retry"
        onRetry={() => refetch()}
        onCancel={onClose}
      />
    )
  }

  if (!isLoading && providers.length === 0) {
    return (
      <div className="flex flex-col h-full pb-8">
        <div className="flex items-center shrink-0 gap-3">
          {onBack && (
            <IconButton onClick={onBack}>
              <BackIcon />
            </IconButton>
          )}
          <div className="grow text-xl tracking-tight text-cw-heading cw-mobile:font-semibold cw-desktop:font-bold leading-6">
            Connect an integration
          </div>
          <IconButton onClick={onClose}>
            <CloseIcon />
          </IconButton>
        </div>

        <div className="flex flex-col items-center justify-center grow gap-3 px-6">
          <div className="flex items-center justify-center rounded-full bg-cw-surface shrink-0 size-14">
            <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
              <path d="M14 3.5L5.25 8.75v10.5L14 24.5l8.75-5.25V8.75L14 3.5z" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinejoin="round" />
              <path d="M14 14v10.5M14 14l8.75-5.25M14 14L5.25 8.75" stroke="var(--color-cw-secondary)" strokeWidth="1.5" />
            </svg>
          </div>
          <div className="text-base text-cw-heading font-semibold leading-5">No integrations configured</div>
          <div className="text-sm text-center text-cw-secondary leading-normal">
            This workspace has no integrations set up yet. Ask your administrator to configure integrations.
          </div>
        </div>

        <Footer />
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full pb-8">
      <div className="flex items-center shrink-0 gap-3">
        {onBack && (
          <IconButton onClick={onBack}>
            <BackIcon />
          </IconButton>
        )}
        <div className="grow text-xl tracking-tight text-cw-heading cw-mobile:font-semibold cw-desktop:font-bold leading-6">
          Connect an integration
        </div>
        <IconButton onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </div>

      <div className="flex items-center cw-mobile:mt-5 cw-desktop:mt-5 shrink-0 cw-mobile:rounded-2.5 cw-desktop:rounded-lg py-3 px-3.5 gap-2.5 bg-cw-surface border border-solid border-cw-border">
        <SearchIcon size={18} className="shrink-0 cw-mobile:hidden" />
        <SearchIcon size={16} className="shrink-0 cw-desktop:hidden" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search integrations..."
          className="text-sm bg-transparent border-none outline-none text-cw-heading leading-4.5 w-full placeholder:text-cw-input-placeholder"
        />
      </div>


      {!search && !isLoading && popular.length > 0 && (
        <div className="flex items-center cw-mobile:mt-4 cw-desktop:mt-5 shrink-0 cw-mobile:gap-2 cw-desktop:flex-col cw-desktop:gap-2.5">
          <div className="cw-mobile:text-xs cw-desktop:text-2xs cw-desktop:tracking-wider cw-desktop:uppercase text-cw-secondary cw-mobile:font-medium cw-desktop:font-semibold cw-mobile:leading-4 cw-desktop:leading-3.5 cw-mobile:mr-1">
            Popular
          </div>

          <div className="hidden cw-desktop:flex flex-wrap gap-2">
            {popular.map((p) => {
              const connected = p.connection_id != null
              return (
                <button
                  key={p.id}
                  onClick={() => connected ? onViewDetail(p) : onSelect(p)}
                  className={`flex items-center rounded-lg py-2.5 px-4 gap-2 bg-cw-surface border border-solid cursor-pointer transition-colors ${
                    connected ? 'border-cw-success/30 hover:border-cw-success/50' : 'border-cw-border hover:border-cw-placeholder'
                  }`}
                >
                  <IntegrationProviderLogo providerName={p.provider ?? ''} className="size-5.5 rounded" />
                  <div className="text-sm text-cw-heading font-medium leading-4.5">{p.display_name || p.provider}</div>
                  {connected && <CheckIcon size={16} />}
                </button>
              )
            })}
          </div>

          <div className="flex cw-desktop:hidden flex-wrap gap-2">
            {popular.slice(0, 3).map((p) => {
              const connected = p.connection_id != null
              return (
                <button
                  key={p.id}
                  onClick={() => connected ? onViewDetail(p) : onSelect(p)}
                  className={`flex items-center rounded-full py-1.5 px-3 gap-1.5 bg-cw-surface border border-solid cursor-pointer transition-colors ${
                    connected ? 'border-cw-success/30 hover:border-cw-success/50' : 'border-cw-border hover:border-cw-placeholder'
                  }`}
                >
                  <div className="text-xs text-cw-heading font-medium leading-4">{p.display_name || p.provider}</div>
                  {connected && <CheckIcon size={14} />}
                </button>
              )
            })}
          </div>
        </div>
      )}

      <div ref={scrollRef} className="flex flex-col cw-mobile:mt-5 cw-desktop:mt-5 grow shrink basis-0 overflow-y-auto cw-mobile:gap-0.5">
        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <SpinnerIcon className="cw-spinner" />
          </div>
        ) : (
          <>
            <div className="hidden cw-desktop:block text-2xs tracking-wider uppercase mb-2 text-cw-secondary font-semibold leading-3.5">
              All Integrations
            </div>
            <div className="relative w-full" style={{ height: `${virtualizer.getTotalSize()}px` }}>
              {virtualizer.getVirtualItems().map((virtualRow) => {
                const p = filtered[virtualRow.index]
                const connected = p.connection_id != null
                return (
                  <button
                    key={p.id}
                    ref={virtualizer.measureElement}
                    data-index={virtualRow.index}
                    onClick={() => connected ? onViewDetail(p) : onSelect(p)}
                    className={`absolute top-0 left-0 w-full flex items-center cw-mobile:py-3.5 cw-desktop:py-3 gap-3.5 bg-transparent border-0 cursor-pointer text-left hover:bg-cw-surface transition-colors ${
                      virtualRow.index < filtered.length - 1 ? 'border-b border-b-solid border-b-cw-divider' : ''
                    }`}
                    style={{ transform: `translateY(${virtualRow.start}px)` }}
                  >
                    <IntegrationProviderLogo providerName={p.provider ?? ''} className="cw-mobile:size-10 cw-desktop:size-9 rounded-lg" />
                    <div className="flex flex-col grow shrink basis-0 gap-0.5">
                      <div className="text-[15px] text-cw-heading font-semibold leading-4.5">{p.display_name || p.provider}</div>
                      {connected ? (
                        <div className="flex items-center gap-1.25">
                          <div className="rounded-full bg-cw-success shrink-0 size-1.5" />
                          <div className="text-xs text-cw-success leading-4">Connected</div>
                        </div>
                      ) : (
                        <div className="text-xs text-cw-secondary leading-4">{formatAuthMode(p.auth_mode ?? '')}</div>
                      )}
                    </div>
                    <ChevronRightIcon />
                  </button>
                )
              })}
            </div>
          </>
        )}
      </div>

      <Footer />
    </div>
  )
}
