import { cn } from "@/lib/utils"

/**
 * Grid + radial-glow backdrop used across every marketing section.
 *
 * The grid lines and glow both rely on CSS values that Tailwind can't express
 * cleanly (multi-stop linear-gradients, `color-mix(in oklch, ...)` for the
 * glow, and a radial-gradient `mask-image` to fade the grid). The utility
 * classes `marketing-grid` and `marketing-glow` encapsulate that CSS so the
 * surrounding markup stays declarative and inline-style free.
 *
 * Positioning variants control where the radial glow is centered — each
 * section historically tuned this by hand (50% 30%, 30% 80%, 70% 50%, …);
 * the named variants bucket those values into reusable presets.
 */
type GlowPosition =
  | "top"
  | "top-center"
  | "hero"
  | "center"
  | "center-high"
  | "bottom"
  | "bottom-center"
  | "left-bottom"
  | "left-bottom-far"
  | "right-center"
  | "right-far"
  | "right-wide"

type GlowIntensity = "sm" | "md" | "md-high" | "lg"

type GridMask =
  | "center"
  | "hero"
  | "bottom"
  | "left-bottom"
  | "left-50"
  | "right-center"
  | "right-50"
  | "right-60"
  | "top-center"

export interface MarketingBackdropProps {
  /** Position of the radial glow. Defaults to `center`. */
  glow?: GlowPosition
  /** Intensity of the glow in `color-mix(primary, transparent)` — sm = 6%, md = 8%, md-high = 10%, lg = 12%. */
  glowIntensity?: GlowIntensity
  /** Position mask applied to the grid (fades edges). Defaults to `center`. */
  grid?: GridMask
  /** Grid cell size. `sm` = 40px, `md` = 60px. Defaults to `sm`. */
  gridSize?: "sm" | "md"
  /** Extra classes — commonly used to restrict `inset-0` reach inside `overflow-hidden` cards. */
  className?: string
}

const glowPositionClass: Record<GlowPosition, string> = {
  top: "marketing-glow-top",
  "top-center": "marketing-glow-top-center",
  hero: "marketing-glow-hero",
  center: "marketing-glow-center",
  "center-high": "marketing-glow-center-high",
  bottom: "marketing-glow-bottom",
  "bottom-center": "marketing-glow-bottom-center",
  "left-bottom": "marketing-glow-left-bottom",
  "left-bottom-far": "marketing-glow-left-bottom-far",
  "right-center": "marketing-glow-right-center",
  "right-far": "marketing-glow-right-far",
  "right-wide": "marketing-glow-right-wide",
}

const glowIntensityClass: Record<GlowIntensity, string> = {
  sm: "marketing-glow-sm",
  md: "marketing-glow-md",
  "md-high": "marketing-glow-md-high",
  lg: "marketing-glow-lg",
}

const gridMaskClass: Record<GridMask, string> = {
  center: "marketing-grid-mask-center",
  hero: "marketing-grid-mask-hero",
  bottom: "marketing-grid-mask-bottom",
  "left-bottom": "marketing-grid-mask-left-bottom",
  "left-50": "marketing-grid-mask-left-50",
  "right-center": "marketing-grid-mask-right-center",
  "right-50": "marketing-grid-mask-right-50",
  "right-60": "marketing-grid-mask-right-60",
  "top-center": "marketing-grid-mask-top-center",
}

export function MarketingBackdrop({
  glow = "center",
  glowIntensity = "md",
  grid = "center",
  gridSize = "sm",
  className,
}: MarketingBackdropProps) {
  return (
    <>
      <div
        aria-hidden="true"
        className={cn(
          "absolute inset-0 pointer-events-none marketing-grid",
          gridSize === "md" && "marketing-grid-lg",
          gridMaskClass[grid],
          className,
        )}
      />
      <div
        aria-hidden="true"
        className={cn(
          "absolute inset-0 pointer-events-none",
          glowPositionClass[glow],
          glowIntensityClass[glowIntensity],
          className,
        )}
      />
    </>
  )
}

/**
 * Standalone glow without the grid — used for card interiors (Pro plan card,
 * billing upgrade card, CTA gradients inside cards).
 */
export function MarketingGlow({
  glow = "center",
  glowIntensity = "md",
  className,
}: Pick<MarketingBackdropProps, "glow" | "glowIntensity" | "className">) {
  return (
    <div
      aria-hidden="true"
      className={cn(
        "absolute inset-0 pointer-events-none",
        glowPositionClass[glow],
        glowIntensityClass[glowIntensity],
        className,
      )}
    />
  )
}
