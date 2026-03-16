/**
 * Reads theme URL params and applies them as CSS variable overrides on the
 * widget root. Runs synchronously during first render (useLayoutEffect) to
 * avoid any flash of default colors.
 *
 * URL params — colors without '#' prefix:
 *   accent, bg, surface, border, heading, body, success, error
 * Non-color params:
 *   font, radius
 */

import { useLayoutEffect } from 'react'

const HEX_RE = /^[0-9a-fA-F]{3}([0-9a-fA-F]{3})?([0-9a-fA-F]{2})?$/

/** Darken a 6-char hex color by a percentage (0-100). */
function darken(hex: string, amount: number): string {
  const c = hex.length === 3
    ? hex[0]+hex[0]+hex[1]+hex[1]+hex[2]+hex[2]
    : hex.slice(0, 6)
  const r = parseInt(c.substring(0, 2), 16)
  const g = parseInt(c.substring(2, 4), 16)
  const b = parseInt(c.substring(4, 6), 16)
  const f = 1 - amount / 100
  return `#${Math.round(r*f).toString(16).padStart(2,'0')}${Math.round(g*f).toString(16).padStart(2,'0')}${Math.round(b*f).toString(16).padStart(2,'0')}`
}

/** Convert hex (without #) to rgba string. */
function rgba(hex: string, alpha: number): string {
  const c = hex.length === 3
    ? hex[0]+hex[0]+hex[1]+hex[1]+hex[2]+hex[2]
    : hex.slice(0, 6)
  const r = parseInt(c.substring(0, 2), 16)
  const g = parseInt(c.substring(2, 4), 16)
  const b = parseInt(c.substring(4, 6), 16)
  return `rgba(${r},${g},${b},${alpha})`
}

interface ParamDef {
  cssVar: string
  derivatives?: (hex: string, set: (v: string, val: string) => void) => void
}

const COLOR_PARAMS: Record<string, ParamDef> = {
  accent: {
    cssVar: '--color-cw-accent',
    derivatives: (hex, set) => {
      set('--color-cw-accent-hover', darken(hex, 12))
      set('--color-cw-accent-active', darken(hex, 22))
      set('--color-cw-accent-subtle', rgba(hex, 0.06))
      set('--color-cw-accent-subtle-border', rgba(hex, 0.12))
      set('--color-cw-logo', `#${hex}`)
    },
  },
  bg:      { cssVar: '--color-cw-bg' },
  surface: { cssVar: '--color-cw-surface' },
  border:  { cssVar: '--color-cw-border' },
  heading: { cssVar: '--color-cw-heading' },
  body:    { cssVar: '--color-cw-body' },
  success: {
    cssVar: '--color-cw-success',
    derivatives: (hex, set) => {
      set('--color-cw-success-bg', rgba(hex, 0.1))
    },
  },
  error: {
    cssVar: '--color-cw-error',
    derivatives: (hex, set) => {
      set('--color-cw-error-bg', rgba(hex, 0.1))
      set('--color-cw-error-hover', darken(hex, 15))
    },
  },
}

/**
 * Parse theme overrides from URL params once, return a stable apply function.
 * Computed outside React so it can also be called before first paint.
 */
function parseOverrides(): ((root: HTMLElement) => void) | null {
  const params = new URLSearchParams(window.location.search)
  const ops: Array<(root: HTMLElement) => void> = []

  for (const [param, def] of Object.entries(COLOR_PARAMS)) {
    const raw = params.get(param)
    if (!raw || !HEX_RE.test(raw)) continue
    const hex = raw
    ops.push((root) => {
      root.style.setProperty(def.cssVar, `#${hex}`)
      def.derivatives?.(hex, (v, val) => root.style.setProperty(v, val))
    })
  }

  const font = params.get('font')
  if (font) {
    ops.push((root) => {
      root.style.setProperty('font-family', `'${font}', system-ui, sans-serif`)
    })
  }

  const radius = params.get('radius')
  if (radius && /^\d+(\.\d+)?(px|rem|em)$/.test(radius)) {
    ops.push((root) => {
      root.style.setProperty('border-radius', radius)
    })
  }

  if (ops.length === 0) return null
  return (root) => { for (const op of ops) op(root) }
}

// Pre-parse on module load so the data is ready before React renders.
const applyOverrides = parseOverrides()

/**
 * Apply URL-param theme overrides to the .connect-widget root element.
 * Uses useLayoutEffect to run before browser paint — no flash.
 */
export function useThemeOverrides() {
  useLayoutEffect(() => {
    if (!applyOverrides) return
    const root = document.querySelector('.connect-widget') as HTMLElement | null
    if (root) applyOverrides(root)
  }, [])
}
