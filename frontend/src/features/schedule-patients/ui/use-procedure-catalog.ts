"use client"

import {useEffect, useRef, useState, type Dispatch, type SetStateAction} from "react"
import {useRouter} from "next/navigation"
import type {Procedure} from "@/entities/procedure"
import {BackendRequestError, sendBackendRequest} from "@/shared/api/backend-client"
import {notify} from "@/shared/lib/notifications"
import type {PatientPlan} from "../model/types"

const CATALOG_UPDATE_KEY = "clinic:procedure-catalog-updated"

export const useProcedureCatalog = (initialProcedures: Procedure[], plans: PatientPlan[], setPlans: Dispatch<SetStateAction<PatientPlan[]>>) => {
  const router = useRouter()
  const [procedureDrafts, setProcedureDrafts] = useState(initialProcedures)
  const procedureDraftVersion = useRef(0)
  const procedureSaveQueue = useRef(Promise.resolve())
  const procedureSaveFailed = useRef(false)
  const [procedureSaveError, setProcedureSaveError] = useState(false)
  const setProcedureDraftsWithVersion: Dispatch<SetStateAction<Procedure[]>> = (next) => {
    procedureDraftVersion.current++
    setProcedureDrafts(next)
  }

  useEffect(() => {
    let active = true
    const version = procedureDraftVersion.current
    // Catalog data arrives from the server component. A server refresh must
    // not overwrite newer local changes that are still being saved.
    void procedureSaveQueue.current.then(() => {
      if (active && version === procedureDraftVersion.current && !procedureSaveFailed.current) {
        setProcedureDrafts(initialProcedures)
      }
    })
    return () => { active = false }
  }, [initialProcedures])

  useEffect(() => {
    // Revalidate through the Next.js server component rather than fetching
    // the catalog again from the browser during hydration.
    const refreshCatalog = () => router.refresh()
    const handleVisibility = () => {
      if (document.visibilityState === "visible") void refreshCatalog()
    }
    window.addEventListener("focus", refreshCatalog)
    document.addEventListener("visibilitychange", handleVisibility)
    const handleStorage = (event: StorageEvent) => {
      if (event.key === CATALOG_UPDATE_KEY) void refreshCatalog()
    }
    window.addEventListener("storage", handleStorage)
    return () => {
      window.removeEventListener("focus", refreshCatalog)
      document.removeEventListener("visibilitychange", handleVisibility)
      window.removeEventListener("storage", handleStorage)
    }
  }, [router])

  const reportSaved = () => {
    procedureSaveFailed.current = false
    setProcedureSaveError(false)
    try { window.localStorage.setItem(CATALOG_UPDATE_KEY, String(Date.now())) } catch { /* Storage is optional. */ }
  }
  const persistProcedureDrafts = (nextProcedures: Procedure[]) => {
    setProcedureDraftsWithVersion(nextProcedures)
    procedureSaveQueue.current = procedureSaveQueue.current.then(async () => {
      try {
        await sendBackendRequest<Procedure[]>("/v1/procedures", "PUT", nextProcedures)
        reportSaved()
      } catch {
        procedureSaveFailed.current = true
        setProcedureSaveError(true)
      }
    })
  }
  const flushProcedureSaves = async () => {
    await procedureSaveQueue.current
    return !procedureSaveFailed.current
  }
  const updateProcedure = (id: string, changes: Partial<Procedure>) => {
    if (changes.isRelaxing === false && procedureDrafts.some((procedure) => procedure.defaultPrerequisiteId === id)) {
      notify.warning("Расслабляющая процедура используется", "Сначала измените связанные процедуры в каталоге, затем снимите признак расслабляющей.")
      return
    }
    if (changes.defaultPrerequisiteId && !procedureDrafts.some((procedure) => procedure.id === changes.defaultPrerequisiteId && procedure.isRelaxing)) {
      notify.warning("Выберите расслабляющую процедуру", "Сначала отметьте подготовительную процедуру как расслабляющую.")
      return
    }
    persistProcedureDrafts(procedureDrafts.map((procedure) => procedure.id === id
      ? {...procedure, ...changes, defaultPrerequisiteId: changes.isRelaxing ? "" : changes.defaultPrerequisiteId ?? procedure.defaultPrerequisiteId}
      : procedure))
    if (changes.isRelaxing === undefined) return
    const hasAffectedLinks = plans.some((plan) =>
      Object.entries(plan.procedurePrerequisites ?? {}).some(([targetId, prerequisiteId]) =>
        changes.isRelaxing ? targetId === id : prerequisiteId === id,
      ) || Object.entries(plan.procedurePrerequisiteOverrides ?? {}).some(([targetId, prerequisiteId]) =>
        changes.isRelaxing ? targetId === id : prerequisiteId === id,
      ),
    )
    if (!hasAffectedLinks) return
    setPlans((all) => all.map((plan) => ({
      ...plan,
      procedurePrerequisites: Object.fromEntries(Object.entries(plan.procedurePrerequisites ?? {}).filter(([targetId, prerequisiteId]) =>
        changes.isRelaxing ? targetId !== id : prerequisiteId !== id,
      )),
      procedurePrerequisiteOverrides: Object.fromEntries(Object.entries(plan.procedurePrerequisiteOverrides ?? {}).filter(([targetId, prerequisiteId]) =>
        changes.isRelaxing ? targetId !== id : prerequisiteId !== id,
      )),
    })))
    notify.warning("Связь процедур обновлена", "Изменение типа процедуры убрало несовместимую связку из курсов пациентов.")
  }
  const addProcedure = () => persistProcedureDrafts([
    ...procedureDrafts,
    {id: crypto.randomUUID(), name: `Процедура ${procedureDrafts.length + 1}`, durationMinutes: 30,
      preparationMinutes: 20, isRelaxing: false, minIntervalDays: 1, defaultWeeklyCount: 1, defaultSessionCount: 1, defaultPrerequisiteId: "", room: ""},
  ])
  const removeProcedure = (id: string) => {
    procedureSaveQueue.current = procedureSaveQueue.current.then(async () => {
      try {
        await sendBackendRequest<null>("/v1/procedures/" + encodeURIComponent(id), "DELETE")
        setProcedureDraftsWithVersion((all) => all.filter((procedure) => procedure.id !== id))
        setPlans((all) => all.map((plan) => ({
          ...plan,
          procedureIds: plan.procedureIds.filter((procedureId) => procedureId !== id),
          procedureCounts: Object.fromEntries(Object.entries(plan.procedureCounts).filter(([procedureId]) => procedureId !== id)),
          procedurePrerequisites: Object.fromEntries(Object.entries(plan.procedurePrerequisites ?? {}).filter(([targetId, prerequisiteId]) => targetId !== id && prerequisiteId !== id)),
          procedurePrerequisiteOverrides: Object.fromEntries(Object.entries(plan.procedurePrerequisiteOverrides ?? {}).filter(([targetId, prerequisiteId]) => targetId !== id && prerequisiteId !== id)),
        })))
        reportSaved()
      } catch (error) {
        procedureSaveFailed.current = true
        if (!(error instanceof BackendRequestError && error.status === 409)) setProcedureSaveError(true)
      }
    })
  }
  const validProcedures = procedureDrafts.every((procedure) =>
    procedure.name.trim() && Number.isInteger(procedure.durationMinutes) && procedure.durationMinutes >= 1 && procedure.durationMinutes <= 420 &&
    Number.isInteger(procedure.preparationMinutes) && procedure.preparationMinutes >= 0 && procedure.durationMinutes + procedure.preparationMinutes <= 420 &&
    Number.isInteger(procedure.minIntervalDays) && procedure.minIntervalDays >= 1 && procedure.minIntervalDays <= 365 &&
    Number.isInteger(procedure.defaultWeeklyCount) && procedure.defaultWeeklyCount >= 1 && procedure.defaultWeeklyCount <= 7 &&
    Number.isInteger(procedure.defaultSessionCount) && procedure.defaultSessionCount >= 1 && procedure.defaultSessionCount <= 365 &&
    (!procedure.defaultPrerequisiteId || !procedure.isRelaxing &&
      procedureDrafts.some((candidate) => candidate.id === procedure.defaultPrerequisiteId && candidate.isRelaxing)) &&
    (procedure.room === undefined || procedure.room.trim().length <= 80),
  )
  useEffect(() => {
    if (!validProcedures) notify.warning("Проверьте параметры процедур", "Длительность: 1–420 мин, перерыв: 0–364 дня, максимум в неделю: 1–7, сеансов по умолчанию: 1–365, кабинет: не более 80 символов.")
  }, [validProcedures])

  return {procedureDrafts, setProcedureDraftsWithVersion, procedureSaveQueue, procedureSaveFailed, procedureSaveError,
    validProcedures, updateProcedure, addProcedure, removeProcedure, flushProcedureSaves}
}
