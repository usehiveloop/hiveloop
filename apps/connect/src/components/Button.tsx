import { type ButtonHTMLAttributes } from 'react'

function ButtonSpinner() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" className="cw-spinner">
      <circle cx="8" cy="8" r="6.5" stroke="currentColor" strokeWidth="2" opacity="0.25" />
      <path d="M8 1.5a6.5 6.5 0 016.37 5.2" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  )
}

const variants = {
  primary:
    'bg-cw-accent border-none text-white font-semibold hover:bg-cw-accent-hover active:bg-cw-accent-active',
  secondary:
    'bg-cw-surface border border-solid border-cw-border text-cw-body font-medium hover:bg-cw-divider',
  danger:
    'bg-cw-error border-none text-white font-semibold hover:bg-cw-error-hover',
}

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof variants
  loading?: boolean
}

export function Button({ variant = 'primary', loading = false, className = '', disabled, children, ...props }: ButtonProps) {
  return (
    <button
      className={`flex items-center justify-center rounded-lg p-3.5 text-[15px] leading-4.5 cursor-pointer transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${variants[variant]} ${className}`}
      disabled={disabled || loading}
      {...props}
    >
      {loading ? <ButtonSpinner /> : children}
    </button>
  )
}
