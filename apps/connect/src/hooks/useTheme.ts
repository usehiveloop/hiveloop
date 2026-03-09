import { useState, useEffect, useCallback } from 'react'
import type { ThemeMode } from '../types'

function getSystemTheme(): 'light' | 'dark' {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function useTheme(initial: ThemeMode = 'system') {
  const [mode, setMode] = useState<ThemeMode>(initial)
  const [resolved, setResolved] = useState<'light' | 'dark'>(
    initial === 'system' ? getSystemTheme() : initial
  )

  useEffect(() => {
    if (mode !== 'system') {
      setResolved(mode)
      return
    }
    setResolved(getSystemTheme())
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = (e: MediaQueryListEvent) => setResolved(e.matches ? 'dark' : 'light')
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [mode])

  const cycle = useCallback(() => {
    setMode((prev) => (prev === 'light' ? 'dark' : prev === 'dark' ? 'system' : 'light'))
  }, [])

  return { mode, resolved, setMode, cycle }
}
