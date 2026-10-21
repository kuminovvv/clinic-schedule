import {useLayoutEffect, useRef, useState} from "react"
import type {CSSProperties} from "react"

const PANEL_HEIGHT_VARIABLE = "--page-driven-scroll-height"

type PageDrivenScrollOptions = {
  /** Distance from the top of the window to the sticky panel, in pixels. */
  topOffset: number
  /** Space below the panel, in pixels. */
  bottomGap?: number
  /** Fit the panel to the visible space below its initial position. */
  heightMode?: "fixed" | "fit-viewport"
  /** Additional scroll distance after the content is exhausted. */
  trailingScrollSpace?: number
  enabled?: boolean
}

/**
 * Uses the document scrollbar to move content inside a sticky, clipped panel.
 *
 * DOM structure: trackRef > panelRef > viewportRef > contentRef.
 * A non-scrolling header may be placed between panelRef and viewportRef.
 * The caller owns the panel's layout and overflow styles.
 */
export const usePageDrivenScroll = ({
  topOffset,
  bottomGap = 0,
  heightMode = "fixed",
  trailingScrollSpace = 0,
  enabled = true,
}: PageDrivenScrollOptions) => {
  const [viewportHeight, setViewportHeight] = useState(0)
  const trackRef = useRef<HTMLDivElement | null>(null)
  const panelRef = useRef<HTMLElement | null>(null)
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const contentRef = useRef<HTMLDivElement | null>(null)
  const panelStyle: CSSProperties = {
    top: topOffset,
    height: `var(${PANEL_HEIGHT_VARIABLE}, calc(100svh - ${topOffset + bottomGap}px))`,
  }

  useLayoutEffect(() => {
    const track = trackRef.current
    const panel = panelRef.current
    const viewport = viewportRef.current
    const content = contentRef.current
    if (!enabled || !track || !panel || !viewport || !content) return

    let animationFrame = 0
    let scrollRange = 0
    let scrollStart = 0

    const applyScrollOffset = () => {
      const scrollOffset = Math.min(scrollRange, Math.max(0, window.scrollY - scrollStart))
      const transform = `translateY(-${scrollOffset}px)`
      if (content.style.transform !== transform) content.style.transform = transform
    }

    const update = () => {
      animationFrame = 0
      const trackTop = track.getBoundingClientRect().top + window.scrollY
      if (heightMode === "fit-viewport") {
        const visibleTop = Math.max(topOffset, trackTop - window.scrollY)
        const nextPanelHeight = Math.max(0, Math.floor(window.innerHeight - visibleTop - bottomGap))
        const nextHeight = `${nextPanelHeight}px`
        if (panel.style.getPropertyValue(PANEL_HEIGHT_VARIABLE) !== nextHeight) {
          panel.style.setProperty(PANEL_HEIGHT_VARIABLE, nextHeight)
        }
      }
      const panelHeight = panel.clientHeight
      const nextViewportHeight = viewport.clientHeight
      setViewportHeight((current) => current === nextViewportHeight ? current : nextViewportHeight)
      scrollRange = Math.max(0, content.scrollHeight - viewport.clientHeight + trailingScrollSpace)
      const trackHeight = `${panelHeight + scrollRange}px`
      if (track.style.height !== trackHeight) track.style.height = trackHeight
      scrollStart = trackTop - topOffset
      applyScrollOffset()
    }

    const scheduleUpdate = () => {
      if (!animationFrame) animationFrame = window.requestAnimationFrame(update)
    }

    const resizeObserver = new ResizeObserver(scheduleUpdate)
    resizeObserver.observe(panel)
    resizeObserver.observe(viewport)
    resizeObserver.observe(content)
    const handleScroll = () => {
      if (heightMode === "fit-viewport") scheduleUpdate()
      else applyScrollOffset()
    }
    window.addEventListener("scroll", handleScroll, {passive: true})
    window.addEventListener("resize", scheduleUpdate)
    update()

    return () => {
      resizeObserver.disconnect()
      window.removeEventListener("scroll", handleScroll)
      window.removeEventListener("resize", scheduleUpdate)
      if (animationFrame) window.cancelAnimationFrame(animationFrame)
      track.style.height = ""
      panel.style.removeProperty(PANEL_HEIGHT_VARIABLE)
      content.style.transform = ""
    }
  }, [enabled, topOffset, bottomGap, heightMode, trailingScrollSpace])

  return {trackRef, panelRef, viewportRef, contentRef, viewportHeight, panelStyle}
}
