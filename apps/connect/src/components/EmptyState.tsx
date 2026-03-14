import { useResolvedTheme } from '../hooks/useResolvedTheme'
import { Button } from './Button'
import { Footer } from './Footer'
import { PageHeader } from './PageHeader'
import { PlusIcon } from './icons'

interface Props {
  onConnect: () => void
  onClose: () => void
}

export function EmptyState({ onConnect, onClose }: Props) {
  const theme = useResolvedTheme()
  const isDark = theme === 'dark'

  return (
    <div className="flex flex-col h-full pb-8">
      <PageHeader title="Connected providers" onClose={onClose} />

      {/* Empty content */}
      <div className={`flex flex-col items-center gap-4 px-6 ${isDark ? 'pt-25 pb-5' : 'pt-12 pb-8'}`}>
        <div
          className={`flex items-center justify-center bg-cw-empty-icon-bg shrink-0 size-14 ${
            isDark ? 'rounded-3.5' : 'rounded-full'
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
        <div className={`text-base text-cw-heading leading-5 ${isDark ? 'font-medium' : 'font-semibold'}`}>
          No providers connected
        </div>
        <div className={`text-sm text-center text-cw-secondary ${isDark ? 'leading-5 max-w-xs' : 'leading-normal'}`}>
          Connect your first LLM provider to start using AI models in {isDark ? 'your application' : 'this app'}.
        </div>
      </div>

      {/* CTA */}
      <Button onClick={onConnect} className={`gap-2 ${isDark ? 'rounded-xl' : ''}`}>
        <PlusIcon />
        Connect a provider
      </Button>

      <Footer />
    </div>
  )
}
