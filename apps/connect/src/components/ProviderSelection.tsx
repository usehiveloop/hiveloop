import { useState, useMemo } from 'react'
import { popularProviderIds } from '../data/providers'
import { useProviders } from '../hooks/useProviders'
import { Error } from './Error'
import { Footer } from './Footer'
import { ProviderLogo } from './ProviderLogo'
import { IconButton } from './IconButton'
import { BackIcon, CloseIcon, SearchIcon, ChevronRightIcon, SpinnerIcon } from './icons'

interface Props {
  onSelect: (providerId: string) => void
  onBack?: () => void
  onClose: () => void
}

export function ProviderSelection({ onSelect, onBack, onClose }: Props) {
  const [search, setSearch] = useState('')
  const { data: providers = [], isLoading, isError, refetch } = useProviders()

  const popular = useMemo(
    () => providers.filter((p) => popularProviderIds.includes(p.id!)),
    [providers]
  )

  const filtered = useMemo(
    () =>
      search.trim()
        ? providers.filter((p) => p.name?.toLowerCase().includes(search.toLowerCase()))
        : providers,
    [search, providers]
  )

  if (isError) {
    return (
      <Error
        title="Unable to load providers"
        message="We couldn't reach the server to load available providers. Please check your connection and try again."
        retryLabel="Retry"
        onRetry={() => refetch()}
        onCancel={onClose}
      />
    )
  }

  return (
    <div className="flex flex-col h-full pb-8">
      {/* Header */}
      <div className="flex items-center shrink-0 gap-3">
        {onBack && (
          <IconButton onClick={onBack}>
            <BackIcon />
          </IconButton>
        )}
        <div className="grow text-xl tracking-tight text-cw-heading cw-mobile:font-semibold cw-desktop:font-bold leading-6">
          Connect a provider
        </div>
        <IconButton onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </div>

      {/* Search */}
      <div className="flex items-center cw-mobile:mt-5 cw-desktop:mt-5 shrink-0 cw-mobile:rounded-2.5 cw-desktop:rounded-lg py-3 px-3.5 gap-2.5 bg-cw-surface border border-solid border-cw-border">
        <SearchIcon size={18} className="shrink-0 cw-mobile:hidden" />
        <SearchIcon size={16} className="shrink-0 cw-desktop:hidden" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search providers..."
          className="text-sm bg-transparent border-none outline-none text-cw-heading leading-4.5 w-full placeholder:text-cw-input-placeholder"
        />
      </div>

      {/* Popular */}
      {!search && !isLoading && (
        <div className="flex items-center cw-mobile:mt-4 cw-desktop:mt-5 shrink-0 cw-mobile:gap-2 cw-desktop:flex-col cw-desktop:gap-2.5">
          <div className="cw-mobile:text-xs cw-desktop:text-2xs cw-desktop:tracking-wider cw-desktop:uppercase text-cw-secondary cw-mobile:font-medium cw-desktop:font-semibold cw-mobile:leading-4 cw-desktop:leading-3.5 cw-mobile:mr-1">
            Popular
          </div>
          {/* Desktop chips */}
          <div className="hidden cw-desktop:flex flex-wrap gap-2">
            {popular.map((p) => (
              <button
                key={p.id}
                onClick={() => onSelect(p.id!)}
                className="flex items-center rounded-lg py-2.5 px-4 gap-2 bg-cw-surface border border-solid border-cw-border cursor-pointer hover:border-cw-placeholder transition-colors"
              >
                <ProviderLogo providerId={p.id!} size="size-5.5" />
                <div className="text-sm text-cw-heading font-medium leading-4.5">{p.name}</div>
              </button>
            ))}
          </div>
          {/* Mobile pills */}
          <div className="flex cw-desktop:hidden flex-wrap gap-2">
            {popular.slice(0, 3).map((p) => (
              <button
                key={p.id}
                onClick={() => onSelect(p.id!)}
                className="flex items-center rounded-full py-1.5 px-3 gap-1.5 bg-cw-surface border border-solid border-cw-border cursor-pointer hover:border-cw-placeholder transition-colors"
              >
                <div className="text-xs text-cw-heading font-medium leading-4">{p.name}</div>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Provider list */}
      <div className="flex flex-col cw-mobile:mt-5 cw-desktop:mt-5 grow shrink basis-0 overflow-y-auto cw-mobile:gap-0.5">
        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <SpinnerIcon className="cw-spinner" />
          </div>
        ) : (
          <>
            <div className="hidden cw-desktop:block text-2xs tracking-wider uppercase mb-2 text-cw-secondary font-semibold leading-3.5">
              All Providers
            </div>
            {filtered.map((p, i) => (
              <button
                key={p.id}
                onClick={() => onSelect(p.id!)}
                className={`flex items-center cw-mobile:py-3.5 cw-desktop:py-3 gap-3.5 bg-transparent border-0 cursor-pointer w-full text-left hover:bg-cw-surface transition-colors ${
                  i < filtered.length - 1 ? 'border-b border-b-solid border-b-cw-divider' : ''
                }`}
              >
                <ProviderLogo providerId={p.id!} size="cw-mobile:size-10 cw-desktop:size-9" />
                <div className="flex flex-col grow shrink basis-0 gap-0.5">
                  <div className="text-[15px] text-cw-heading font-semibold leading-4.5">{p.name}</div>
                  <div className="text-xs text-cw-secondary leading-4">
                    {p.model_count} {p.model_count === 1 ? 'model' : 'models'}
                  </div>
                </div>
                <ChevronRightIcon />
              </button>
            ))}
          </>
        )}
      </div>

      <Footer />
    </div>
  )
}
