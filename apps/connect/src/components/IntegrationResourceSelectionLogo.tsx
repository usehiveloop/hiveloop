import { IntegrationProviderLogo } from './IntegrationProviderLogo'

interface Props {
  providerName: string
  className?: string
}

export function IntegrationResourceSelectionLogo({ providerName, className = 'size-10 rounded-lg' }: Props) {
  return <IntegrationProviderLogo providerName={providerName} className={className} />
}
