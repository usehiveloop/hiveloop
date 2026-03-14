import { type ButtonHTMLAttributes } from 'react'

export function IconButton({ className = '', ...props }: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      className={`cursor-pointer bg-transparent border-none p-0 ${className}`}
      {...props}
    />
  )
}
