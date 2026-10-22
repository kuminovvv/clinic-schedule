"use client"

import {useCallback, useEffect, useRef, useState} from "react"
import {CALENDAR_SETTINGS_CHANGE_EVENT, type CalendarSettings, validateWorkWeek} from "@/entities/calendar-settings"
import {sendBackendRequest} from "@/shared/api/backend-client"

type SaveStatus = "saved" | "pending" | "saving" | "error"

/** Debounced and serialized writes: an older response never replaces a newer edit. */
export function useCalendarSettingsAutosave(initial: CalendarSettings) {
  const [draft, setDraft] = useState(initial)
  const [status, setStatus] = useState<SaveStatus>("saved")
  const [retry, setRetry] = useState(0)
  const [savedHash, setSavedHash] = useState(JSON.stringify(initial))
  const requestVersion = useRef(0)
  const queue = useRef<Promise<void>>(Promise.resolve())
  const error = validateWorkWeek(draft.workWeek)
  const current = JSON.stringify(draft)

  const update = useCallback((apply: (previous: CalendarSettings) => CalendarSettings) => {
    setDraft(apply)
    setStatus("pending")
  }, [])

  useEffect(() => {
    const version = ++requestVersion.current
    if (current === savedHash) return
    if (error) return
    const snapshot = draft
    const timer = window.setTimeout(() => {
      queue.current = queue.current.catch(() => {}).then(async () => {
        if (version !== requestVersion.current) return
        setStatus("saving")
        try {
          const response = await sendBackendRequest<CalendarSettings>("/v1/calendar-settings", "PUT", snapshot)
          if (!response) throw new Error("Empty settings response")
          setSavedHash(JSON.stringify(response))
          window.dispatchEvent(new Event(CALENDAR_SETTINGS_CHANGE_EVENT))
          if (version === requestVersion.current) setStatus("saved")
        } catch {
          if (version === requestVersion.current) setStatus("error")
        }
      })
    }, 550)
    return () => {
      window.clearTimeout(timer)
    }
  }, [current, draft, error, retry, savedHash])

  return {
    settings: draft,
    update,
    error,
    status: error ? "invalid" as const : current === savedHash ? "saved" as const : status,
    retry: () => {setStatus("pending"); setRetry((value) => value + 1)},
  }
}
