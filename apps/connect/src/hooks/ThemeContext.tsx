import { createContext, useContext } from 'react'

const ThemeContext = createContext<'light' | 'dark'>('light')

export const ThemeProvider = ThemeContext.Provider

export function useResolvedTheme() {
  return useContext(ThemeContext)
}
