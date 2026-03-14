import { Button } from './Button'
import { Footer } from './Footer'
import { WarningIcon } from './icons'

interface Props {
  title?: string
  message?: string
  retryLabel?: string
  onRetry: () => void
  onCancel: () => void
}

export function Error({
  title = 'Connection failed',
  message = 'The API key could not be validated. Please check your key and try again.',
  retryLabel = 'Try again',
  onRetry,
  onCancel,
}: Props) {
  return (
    <div className="flex flex-col items-center justify-center h-full py-7 px-12 gap-3">
      <div className="flex items-center justify-center rounded-full bg-cw-error-bg shrink-0 size-14">
        <WarningIcon />
      </div>
      <div className="text-xl text-cw-heading font-bold leading-6">{title}</div>
      <div className="text-sm text-center leading-normal text-cw-secondary">{message}</div>
      <div className="flex flex-col w-full mt-3 gap-2.5">
        <Button onClick={onRetry}>{retryLabel}</Button>
        <Button variant="secondary" onClick={onCancel}>Cancel</Button>
      </div>
      <Footer />
    </div>
  )
}
