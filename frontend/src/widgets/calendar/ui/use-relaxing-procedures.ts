"use client"

import {useEffect, useState} from "react"
import type {Procedure} from "@/entities/procedure"
import {getClientData} from "@/shared/api/backend-client"

export const useRelaxingProcedures = () => {
  const [relaxingIds, setRelaxingIds] = useState<Set<string>>(new Set())
  useEffect(() => {
    let active = true
    const refresh = async () => {
      try {
        const procedures = await getClientData<Procedure[]>("/v1/procedures")
        if (active) setRelaxingIds(new Set(procedures.filter((procedure) => procedure.isRelaxing).map((procedure) => procedure.id)))
      } catch {
        // Retain the last known catalog on a network failure.
      }
    }
    const onVisibility = () => { if (document.visibilityState === "visible") void refresh() }
    void refresh()
    window.addEventListener("focus", refresh)
    document.addEventListener("visibilitychange", onVisibility)
    return () => {
      active = false
      window.removeEventListener("focus", refresh)
      document.removeEventListener("visibilitychange", onVisibility)
    }
  }, [])
  return relaxingIds
}
