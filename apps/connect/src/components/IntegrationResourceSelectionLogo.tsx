import { ProviderLogo } from './ProviderLogo'

interface Props {
  providerName: string
  size?: string
  rounded?: string
}

export function IntegrationResourceSelectionLogo({ providerName, size = 'size-10', rounded = 'rounded-lg' }: Props) {
  return <ProviderLogo providerId={providerName} size={size} rounded={rounded} />
}
