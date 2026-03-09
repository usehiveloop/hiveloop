import { useResolvedTheme } from '../hooks/ThemeContext'
import { Footer } from './Footer'

interface Props {
  onConnect: () => void
  onClose: () => void
}

export function EmptyState({ onConnect, onClose }: Props) {
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

      {/* Empty content */}
      <div className={`flex flex-col items-center gap-4 px-6 ${isDark ? 'pt-[100px] pb-5' : 'pt-12 pb-8'}`}>
        <div
          className={`flex items-center justify-center bg-cw-empty-icon-bg shrink-0 size-14 ${
            isDark ? 'rounded-[14px]' : 'rounded-[50%]'
          }`}
        >
          {isDark ? (
            <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
              <path d="M7 21V7a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v14" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
              <path d="M5 21h18" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
              <path d="M11 10h6M11 14h4" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
            </svg>
          ) : (
            <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
              <path d="M14 3.5L5.25 8.75v10.5L14 24.5l8.75-5.25V8.75L14 3.5z" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinejoin="round" />
              <path d="M14 14v10.5M14 14l8.75-5.25M14 14L5.25 8.75" stroke="var(--color-cw-secondary)" strokeWidth="1.5" />
            </svg>
          )}
        </div>
        <div className={`text-[16px] text-cw-heading leading-5 ${isDark ? 'font-medium' : 'font-semibold'}`}>
          No providers connected
        </div>
        <div className={`text-[14px] text-center text-cw-secondary ${isDark ? 'leading-5 max-w-[320px]' : 'leading-[150%]'}`}>
          Connect your first LLM provider to start using AI models in {isDark ? 'your application' : 'this app'}.
        </div>
      </div>

      {/* CTA */}
      <button
        onClick={onConnect}
        className={`flex items-center justify-center gap-2 bg-cw-accent p-3.5 cursor-pointer border-none hover:bg-cw-accent-hover active:bg-cw-accent-active transition-colors ${
          isDark ? 'rounded-[12px]' : 'rounded-lg'
        }`}
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
          <path d="M8 3v10M3 8h10" stroke="#FFFFFF" strokeWidth="1.5" strokeLinecap="round" />
        </svg>
        <div className={`text-white leading-[18px] ${isDark ? 'text-[14px] font-medium' : 'text-[15px] font-semibold'}`}>
          Connect a provider
        </div>
      </button>

      <Footer />
    </div>
  )
}
