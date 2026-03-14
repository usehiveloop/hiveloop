interface IconProps {
  size?: number
  className?: string
}

export function CloseIcon({ size = 20, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
      <path d="M15 5L5 15M5 5l10 10" stroke="var(--color-cw-secondary)" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  )
}

export function BackIcon({ size = 20, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
      <path d="M13 4L7 10l6 6" stroke="var(--color-cw-icon-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function ChevronRightIcon({ size = 16, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={className}>
      <path d="M6 4l4 4-4 4" stroke="var(--color-cw-placeholder)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function SearchIcon({ size = 18, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 18 18" fill="none" className={className}>
      <circle cx="8" cy="8" r="5.5" stroke="var(--color-cw-placeholder)" strokeWidth="1.5" />
      <path d="M12.5 12.5L16 16" stroke="var(--color-cw-placeholder)" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  )
}

export function ShieldIcon({ size = 18, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 18 18" fill="none" className={className}>
      <path d="M9 1.5l-6 3v4.5c0 3.86 2.56 7.47 6 8.5 3.44-1.03 6-4.64 6-8.5V4.5l-6-3z" stroke="var(--color-cw-accent)" strokeWidth="1.2" />
      <path d="M6.5 9l1.75 1.75L11.5 7.5" stroke="var(--color-cw-accent)" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function PlusIcon({ size = 16, className, stroke = '#FFFFFF' }: IconProps & { stroke?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={className}>
      <path d="M8 3v10M3 8h10" stroke={stroke} strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  )
}

export function EyeIcon({ size = 18, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 18 18" fill="none" className={className}>
      <path d="M1.5 9s3-5.25 7.5-5.25S16.5 9 16.5 9s-3 5.25-7.5 5.25S1.5 9 1.5 9z" stroke="var(--color-cw-secondary)" strokeWidth="1.2" />
      <circle cx="9" cy="9" r="2.25" stroke="var(--color-cw-secondary)" strokeWidth="1.2" />
    </svg>
  )
}

export function EyeOffIcon({ size = 18, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 18 18" fill="none" className={className}>
      <path d="M2.5 2.5l13 13M7.36 7.36a2.5 2.5 0 003.28 3.28" stroke="var(--color-cw-placeholder)" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M1.5 9s3-5.25 7.5-5.25c1.24 0 2.36.35 3.33.88M14.7 11.1C15.82 10.07 16.5 9 16.5 9s-3-5.25-7.5-5.25" stroke="var(--color-cw-placeholder)" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function WarningIcon({ size = 28, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 28 28" fill="none" className={className}>
      <path d="M14 9v6M14 19h.01" stroke="var(--color-cw-error)" strokeWidth="2.5" strokeLinecap="round" />
    </svg>
  )
}

export function CheckIcon({ size = 28, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 28 28" fill="none" className={className}>
      <path d="M8 14.5l4 4 8-8" stroke="var(--color-cw-success)" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function SpinnerIcon({ size = 24, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" className={className}>
      <circle cx="12" cy="12" r="10" stroke="var(--color-cw-border)" strokeWidth="2.5" />
      <path d="M12 2a10 10 0 019.8 8" stroke="var(--color-cw-accent)" strokeWidth="2.5" strokeLinecap="round" />
    </svg>
  )
}
