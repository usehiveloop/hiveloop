import type { IntegrationProvider } from '../types'
import { Button } from './Button'
import { Footer } from './Footer'
import { IntegrationProviderLogo } from './IntegrationProviderLogo'
import { CheckIcon } from './icons'

interface Props {
  integration: IntegrationProvider
  onDone: () => void
}

export function ResourceSelectionSuccess({ integration, onDone }: Props) {
  const name = integration.display_name || integration.provider || ''

  return (
    <div className="flex flex-col items-center justify-center h-full py-7 px-12 gap-4">
      <div className="flex items-center justify-center rounded-full bg-cw-success-bg shrink-0 size-16 relative">
        <CheckIcon size={32} />
        <div className="absolute -right-2 -bottom-2">
          <IntegrationProviderLogo
            providerName={integration.provider ?? ''}
            className="size-8 rounded-lg"
          />
        </div>
      </div>

      <div className="text-xl text-cw-heading font-bold leading-6">Selection saved</div>
      <div className="text-sm text-center leading-normal text-cw-secondary">
        Resource permissions for {name} have been updated.
      </div>

      <div className="flex flex-col w-full gap-2 mt-3">
        <Button onClick={onDone} variant="primary">
          Done
        </Button>
      </div>

      <Footer />
    </div>
  )
}
