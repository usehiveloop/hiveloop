import { useContext } from 'react'
import { ThemeContext } from './themeContextDef'

export function useResolvedTheme() {
  return useContext(ThemeContext)
}
