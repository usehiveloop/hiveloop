import { useProviders } from '../hooks/useProviders'
import { Button } from './Button'
import { Footer } from './Footer'
import { CheckIcon } from './icons'

interface Props {
  providerId: string
  title?: string
  message?: string
  doneLabel?: string
  onDone: () => void
}

export function Success({ providerId, title, message, doneLabel = 'Done', onDone }: Props) {
  const { data: providers = [] } = useProviders()
  const provider = providers.find((p) => p.id === providerId)
  const providerName = provider?.name ?? providerId

  return (
    <div className="flex flex-col items-center justify-center h-full py-7 px-12 gap-3">
      <div className="flex items-center justify-center rounded-full bg-cw-success-bg shrink-0 size-14">
        <CheckIcon />
      </div>
      <div className="text-xl text-cw-heading font-bold leading-6">{title ?? 'Connected'}</div>
      <div className="text-sm text-center leading-normal text-cw-secondary">
        {message ?? `${providerName} is ready to use. Your API key has been encrypted and stored securely.`}
      </div>
      <Button onClick={onDone} className="w-full mt-3">{doneLabel}</Button>
      <Footer />
    </div>
  )
}
