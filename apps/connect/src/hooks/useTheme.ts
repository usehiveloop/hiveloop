import { useState, useCallback, useSyncExternalStore } from 'react'
import type { ThemeMode } from '../types'

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function subscribeSystemTheme(callback: () => void) {
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  mq.addEventListener('change', callback)
  return () => mq.removeEventListener('change', callback)
}

function getServerTheme(): 'light' | 'dark' {
  return 'light'
}

export function useTheme(initial: ThemeMode = 'system') {
  const [mode, setMode] = useState<ThemeMode>(initial)

  const systemTheme = useSyncExternalStore(
    subscribeSystemTheme,
    getSystemTheme,
    getServerTheme,
  )

  const resolved = mode === 'system' ? systemTheme : mode

  const cycle = useCallback(() => {
    setMode((prev) => (prev === 'light' ? 'dark' : prev === 'dark' ? 'system' : 'light'))
  }, [])

  return { mode, resolved, setMode, cycle }
}
