import { getProviderColor } from '../data/brand-colors'

interface Props {
  providerId: string
  size: string
  rounded?: string
}

export function ProviderLogo({ providerId, size, rounded = 'rounded-lg' }: Props) {
  return (
    <div
      className={`shrink-0 ${rounded} ${size} flex items-center justify-center`}
      style={{ backgroundColor: getProviderColor(providerId) }}
    >
      <img
        src={`/logos/${providerId}.svg`}
        alt=""
        className="w-3/5 h-3/5 object-contain"
      />
    </div>
  )
}
