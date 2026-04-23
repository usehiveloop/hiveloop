"use client"

import { useEffect, useRef } from "react"

/**
 * Fixed-position progress bar that tracks how far the reader has scrolled
 * through the page. Lives in its own client component so the slug page can
 * stay a server component (and keep its static-generation properties via
 * generateStaticParams).
 *
 * Why direct DOM ref mutation instead of React state:
 *
 * Going through React's state → reconciliation → commit pipeline adds
 * milliseconds of visible lag on every scroll event, which makes the bar
 * feel like it's chasing the cursor instead of being glued to it. Writing
 * directly to `ref.current.style.width` skips React entirely and updates
 * the DOM in microseconds.
 *
 * Why no CSS transition:
 *
 * A CSS `transition: width Xms` smooths the width animation over X
 * milliseconds, which means even after the value updates instantly the
 * browser is still animating it. That's the lag you feel. The transition
 * is removed so each frame's width is the exact current scroll position,
 * not an interpolation toward it.
 *
 * Why rAF coalescing:
 *
 * Scroll events fire faster than the screen refreshes, especially on
 * touchpads with smooth scrolling. requestAnimationFrame collapses
 * multiple events into one update per browser frame, which is the
 * physical maximum update rate the user can perceive anyway. Without
 * rAF you'd be doing redundant DOM writes that the browser would just
 * batch and discard.
 */
export function ReadingProgress() {
  const barRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let rafId = 0

    const update = () => {
      const scrollTop = window.scrollY
      const documentHeight =
        document.documentElement.scrollHeight - window.innerHeight
      const ratio =
        documentHeight <= 0
          ? 0
          : Math.min(1, Math.max(0, scrollTop / documentHeight))
      if (barRef.current) {
        barRef.current.style.width = `${ratio * 100}%`
      }
    }

    const onScroll = () => {
      // Coalesce — at most one DOM write per animation frame.
      if (rafId) return
      rafId = window.requestAnimationFrame(() => {
        rafId = 0
        update()
      })
    }

    update() // initial paint — handles back/forward restores at non-zero scroll
    window.addEventListener("scroll", onScroll, { passive: true })
    window.addEventListener("resize", onScroll, { passive: true })

    return () => {
      window.removeEventListener("scroll", onScroll)
      window.removeEventListener("resize", onScroll)
      if (rafId) window.cancelAnimationFrame(rafId)
    }
  }, [])

  return (
    <div className="fixed top-0 left-0 right-0 z-200 h-0.5 bg-border">
      {/*
        Width starts at 0 via `w-0`; the effect below mutates
        `ref.current.style.width` directly on every scroll frame — bypassing
        React so the update lands in microseconds instead of waiting for a
        reconciliation pass. The runtime width can't be expressed as a static
        Tailwind class.
      */}
      <div ref={barRef} className="h-full w-0 bg-primary" />
    </div>
  )
}
