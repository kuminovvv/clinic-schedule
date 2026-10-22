import * as React from "react"

const MOBILE_BREAKPOINT = 768
const mobileQuery = `(max-width: ${MOBILE_BREAKPOINT - 1}px)`

const subscribeToMobile = (onChange: () => void) => {
  const mediaQuery = window.matchMedia(mobileQuery)
  mediaQuery.addEventListener("change", onChange)
  return () => mediaQuery.removeEventListener("change", onChange)
}

const getMobileSnapshot = () => window.matchMedia(mobileQuery).matches
const getServerSnapshot = () => false

export const useIsMobile = () => {
  return React.useSyncExternalStore(subscribeToMobile, getMobileSnapshot, getServerSnapshot)
}
