import { useState, useMemo } from 'react'
import { providers, popularProviderIds } from '../data/providers'
import { Footer } from './Footer'

interface Props {
  onSelect: (providerId: string) => void
  onClose: () => void
}

export function ProviderSelection({ onSelect, onClose }: Props) {
  const [search, setSearch] = useState('')

  const popular = useMemo(
    () => providers.filter((p) => popularProviderIds.includes(p.id)),
    []
  )

  const filtered = useMemo(
    () =>
      search.trim()
        ? providers.filter((p) =>
            p.name.toLowerCase().includes(search.toLowerCase()) ||
            p.models.toLowerCase().includes(search.toLowerCase())
          )
        : providers,
    [search]
  )

  return (
    <div className="flex flex-col h-full pb-8">
      {/* Header */}
      <div className="flex items-center justify-between shrink-0">
        <div className="text-[20px] tracking-[-0.02em] text-cw-heading cw-mobile:font-semibold cw-desktop:font-bold leading-6">
          Connect a provider
        </div>
        <button onClick={onClose} className="cursor-pointer bg-transparent border-none p-0">
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
            <path d="M15 5L5 15M5 5l10 10" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      </div>

      {/* Search */}
      <div className="flex items-center cw-mobile:mt-5 cw-desktop:mt-5 shrink-0 cw-mobile:rounded-[10px] cw-desktop:rounded-lg py-3 px-3.5 gap-2.5 bg-cw-surface border border-solid border-cw-border">
        <svg width="18" height="18" viewBox="0 0 18 18" fill="none" className="shrink-0 cw-mobile:hidden">
          <circle cx="8" cy="8" r="5.5" stroke="var(--color-cw-placeholder)" strokeWidth="1.5" />
          <path d="M12.5 12.5L16 16" stroke="var(--color-cw-placeholder)" strokeWidth="1.5" strokeLinecap="round" />
        </svg>
        <svg width="16" height="16" viewBox="0 0 18 18" fill="none" className="shrink-0 cw-desktop:hidden">
          <circle cx="8" cy="8" r="5.5" stroke="var(--color-cw-placeholder)" strokeWidth="1.5" />
          <path d="M12.5 12.5L16 16" stroke="var(--color-cw-placeholder)" strokeWidth="1.5" strokeLinecap="round" />
        </svg>
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search providers..."
          className="text-[14px] bg-transparent border-none outline-none text-cw-heading leading-[18px] w-full placeholder:text-cw-input-placeholder"
        />
      </div>

      {/* Popular */}
      {!search && (
        <div className="flex items-center cw-mobile:mt-4 cw-desktop:mt-5 shrink-0 cw-mobile:gap-2 cw-desktop:flex-col cw-desktop:gap-2.5">
          {/* Label — inline on mobile, block on desktop */}
          <div className="cw-mobile:text-[12px] cw-desktop:text-[11px] cw-desktop:tracking-[0.06em] cw-desktop:uppercase text-cw-secondary cw-mobile:font-medium cw-desktop:font-semibold cw-mobile:leading-4 cw-desktop:leading-3.5 cw-mobile:mr-1">
            Popular
          </div>
          {/* Desktop: card chips with color squares */}
          <div className="hidden cw-desktop:flex flex-wrap gap-2">
            {popular.map((p) => (
              <button
                key={p.id}
                onClick={() => onSelect(p.id)}
                className="flex items-center rounded-lg py-2.5 px-4 gap-2 bg-cw-surface border border-solid border-cw-border cursor-pointer hover:border-cw-placeholder transition-colors"
              >
                <div className={`w-[22px] h-[22px] rounded-[5px] shrink-0 ${p.colorClass}`} />
                <div className="text-[14px] text-cw-heading font-medium leading-[18px]">
                  {p.name}
                </div>
              </button>
            ))}
          </div>
          {/* Mobile: pill chips, text only */}
          <div className="flex cw-desktop:hidden flex-wrap gap-2">
            {popular.slice(0, 3).map((p) => (
              <button
                key={p.id}
                onClick={() => onSelect(p.id)}
                className="flex items-center rounded-full py-1.5 px-3 gap-1.5 bg-cw-surface border border-solid border-cw-border cursor-pointer hover:border-cw-placeholder transition-colors"
              >
                <div className="text-[12px] text-cw-heading font-medium leading-4">
                  {p.name === 'Google Gemini' ? 'Gemini' : p.name}
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Provider list */}
      <div className="flex flex-col cw-mobile:mt-5 cw-desktop:mt-5 grow shrink basis-[0%] overflow-y-auto cw-mobile:gap-0.5">
        <div className="hidden cw-desktop:block text-[11px] tracking-[0.06em] uppercase mb-2 text-cw-secondary font-semibold leading-3.5">
          All Providers
        </div>
        {filtered.map((p, i) => (
          <button
            key={p.id}
            onClick={() => onSelect(p.id)}
            className={`flex items-center cw-mobile:py-3.5 cw-desktop:py-3 gap-3.5 bg-transparent border-0 cursor-pointer w-full text-left hover:bg-cw-surface transition-colors ${
              i < filtered.length - 1 ? 'border-b border-b-solid border-b-cw-divider' : ''
            }`}
          >
            <div className={`shrink-0 rounded-lg cw-mobile:size-10 cw-desktop:size-9 ${p.colorClass}`} />
            <div className="flex flex-col grow shrink basis-[0%] gap-0.5">
              <div className="text-[15px] text-cw-heading font-semibold leading-[18px]">
                {p.name}
              </div>
              <div className="text-[12px] text-cw-secondary leading-4">
                {p.models}
              </div>
            </div>
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
              <path d="M6 4l4 4-4 4" stroke="var(--color-cw-placeholder)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
        ))}
      </div>

      <Footer />
    </div>
  )
}
