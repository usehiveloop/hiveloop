import { IconButton } from './IconButton'
import { CloseIcon } from './icons'

interface Props {
  title: string
  onClose: () => void
}

export function PageHeader({ title, onClose }: Props) {
  return (
    <div className="flex items-center justify-between shrink-0">
      <div className="text-xl tracking-tight text-cw-heading font-bold leading-6">
        {title}
      </div>
      <IconButton onClick={onClose}>
        <CloseIcon />
      </IconButton>
    </div>
  )
}
