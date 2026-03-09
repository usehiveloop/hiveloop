import { useState } from 'react'
import { providers, consoleUrls } from '../data/providers'
import { Footer } from './Footer'

interface Props {
  providerId: string
  onSubmit: () => void
  onBack: () => void
  onClose: () => void
}

export function ApiKeyInput({ providerId, onSubmit, onBack, onClose }: Props) {
  const provider = providers.find((p) => p.id === providerId)!
  const [apiKey, setApiKey] = useState('')
  const [label, setLabel] = useState('')
  const [showKey, setShowKey] = useState(false)

  return (
    <div className="flex flex-col h-full pb-8">
      {/* Header */}
      <div className="flex items-center shrink-0 gap-3.5">
        <button onClick={onBack} className="cursor-pointer bg-transparent border-none p-0">
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
            <path d="M13 4L7 10l6 6" stroke="var(--color-cw-icon-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </button>
        <div className={`shrink-0 rounded-[7px] size-8 ${provider.colorClass}`} />
        <div className="text-[18px] grow shrink basis-[0%] text-cw-heading cw-mobile:font-semibold cw-desktop:font-bold leading-[22px]">
          {provider.name}
        </div>
        {/* Close — hidden on mobile */}
        <button onClick={onClose} className="cursor-pointer bg-transparent border-none p-0 cw-mobile:hidden">
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
            <path d="M15 5L5 15M5 5l10 10" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      </div>

      {/* Form */}
      <div className="flex flex-col cw-mobile:mt-8 cw-desktop:mt-7 shrink-0 cw-mobile:gap-5 cw-desktop:gap-4">
        {/* API Key field */}
        <div className="flex flex-col gap-1.5">
          <div className="text-[13px] text-cw-heading cw-mobile:font-medium cw-desktop:font-semibold leading-4">
            API Key
          </div>
          <div className="flex items-center cw-mobile:rounded-[10px] cw-desktop:rounded-lg py-3 px-3.5 gap-2 cw-mobile:bg-cw-bg cw-mobile:border cw-mobile:border-solid cw-mobile:border-cw-border cw-desktop:bg-cw-surface cw-desktop:border cw-desktop:border-solid cw-desktop:border-cw-border focus-within:border-cw-accent focus-within:ring-2 focus-within:ring-cw-accent-subtle transition-colors">
            <input
              type={showKey ? 'text' : 'password'}
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={`Paste your ${provider.name} API key`}
              className="text-[14px] grow shrink basis-[0%] bg-transparent border-none outline-none text-cw-heading leading-[18px] placeholder:text-cw-input-placeholder"
            />
            <button
              onClick={() => setShowKey(!showKey)}
              className="cursor-pointer bg-transparent border-none p-0 shrink-0"
            >
              <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
                {showKey ? (
                  <>
                    <path d="M2.5 2.5l13 13M7.36 7.36a2.5 2.5 0 003.28 3.28" stroke="var(--color-cw-placeholder)" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
                    <path d="M1.5 9s3-5.25 7.5-5.25c1.24 0 2.36.35 3.33.88M14.7 11.1C15.82 10.07 16.5 9 16.5 9s-3-5.25-7.5-5.25" stroke="var(--color-cw-placeholder)" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
                  </>
                ) : (
                  <>
                    <path d="M1.5 9s3-5.25 7.5-5.25S16.5 9 16.5 9s-3 5.25-7.5 5.25S1.5 9 1.5 9z" stroke="var(--color-cw-secondary)" strokeWidth="1.2" />
                    <circle cx="9" cy="9" r="2.25" stroke="var(--color-cw-secondary)" strokeWidth="1.2" />
                  </>
                )}
              </svg>
            </button>
          </div>
          {/* Helper text — desktop: link style, mobile: plain description */}
          <div className="cw-desktop:block cw-mobile:hidden">
            <span className="text-[12px] text-cw-secondary leading-4">
              Find your API key at{' '}
            </span>
            <span className="text-[12px] text-cw-accent leading-4">
              {consoleUrls[providerId]}
            </span>
          </div>
          <div className="cw-mobile:block cw-desktop:hidden text-[12px] text-cw-secondary leading-4">
            Paste your {provider.name} API key. It will be encrypted before storage.
          </div>
        </div>

        {/* Label field */}
        <div className="flex flex-col gap-1.5">
          {/* Desktop: Label + — optional; Mobile: Label (optional) */}
          <div className="cw-mobile:hidden flex items-baseline gap-1.5">
            <div className="text-[13px] text-cw-heading font-semibold leading-4">
              Label
            </div>
            <div className="text-[12px] text-cw-input-placeholder leading-4">
              — optional
            </div>
          </div>
          <div className="cw-desktop:hidden text-[13px] text-cw-heading font-medium leading-4">
            Label (optional)
          </div>
          <div className="flex items-center cw-mobile:rounded-[10px] cw-desktop:rounded-lg py-3 px-3.5 cw-mobile:bg-cw-bg cw-mobile:border cw-mobile:border-solid cw-mobile:border-cw-border cw-desktop:bg-cw-surface cw-desktop:border cw-desktop:border-solid cw-desktop:border-cw-border focus-within:border-cw-accent focus-within:ring-2 focus-within:ring-cw-accent-subtle transition-colors">
            <input
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="e.g. Production key"
              className="text-[14px] bg-transparent border-none outline-none text-cw-heading leading-[18px] w-full placeholder:text-cw-input-placeholder"
            />
          </div>
        </div>
      </div>

      {/* Security callout — desktop: accent subtle bg; mobile: surface bg with title/body */}
      <div className="cw-mobile:hidden flex items-start mt-2 shrink-0 rounded-lg gap-2.5 bg-cw-accent-subtle border border-solid border-cw-accent-subtle-border p-3.5">
        <svg width="18" height="18" viewBox="0 0 18 18" fill="none" className="shrink-0 mt-px">
          <path d="M9 1.5l-6 3v4.5c0 3.86 2.56 7.47 6 8.5 3.44-1.03 6-4.64 6-8.5V4.5l-6-3z" stroke="var(--color-cw-accent)" strokeWidth="1.2" />
          <path d="M6.5 9l1.75 1.75L11.5 7.5" stroke="var(--color-cw-accent)" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
        <div className="text-[13px] leading-[150%] text-cw-body">
          Your key is encrypted end-to-end with AES-256-GCM and never stored in plaintext.
        </div>
      </div>
      <div className="cw-desktop:hidden flex items-start mt-2 shrink-0 rounded-[10px] gap-2.5 bg-cw-surface p-3.5">
        <svg width="16" height="16" viewBox="0 0 18 18" fill="none" className="shrink-0 mt-px">
          <path d="M9 1.5l-6 3v4.5c0 3.86 2.56 7.47 6 8.5 3.44-1.03 6-4.64 6-8.5V4.5l-6-3z" stroke="var(--color-cw-accent)" strokeWidth="1.2" />
          <path d="M6.5 9l1.75 1.75L11.5 7.5" stroke="var(--color-cw-accent)" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
        <div className="flex flex-col gap-0.5">
          <div className="text-[13px] text-cw-heading font-medium leading-4">
            End-to-end encrypted
          </div>
          <div className="text-[12px] text-cw-secondary leading-4">
            Your key is encrypted with AES-256 before leaving this device.
          </div>
        </div>
      </div>

      {/* Connect button */}
      <button
        onClick={onSubmit}
        disabled={!apiKey.trim()}
        className="flex items-center justify-center cw-mobile:mt-6 cw-desktop:mt-4 shrink-0 cw-mobile:rounded-[10px] cw-desktop:rounded-lg bg-cw-accent p-3.5 cursor-pointer border-none hover:bg-cw-accent-hover active:bg-cw-accent-active disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
      >
        <div className="text-[15px] text-white cw-mobile:font-medium cw-desktop:font-semibold leading-[18px]">
          <span className="cw-desktop:hidden">Connect {provider.name}</span>
          <span className="cw-mobile:hidden">Connect</span>
        </div>
      </button>

      <Footer />
    </div>
  )
}
