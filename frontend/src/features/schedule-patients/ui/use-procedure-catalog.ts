"use client"

import {useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction} from "react"
import {useRouter} from "next/navigation"
import type {Procedure} from "@/entities/procedure"
import {BackendRequestError, sendBackendRequest} from "@/shared/api/backend-client"
import {notify} from "@/shared/lib/notifications"
import type {PatientPlan} from "../model/types"

const CATALOG_UPDATE_KEY = "clinic:procedure-catalog-updated"
const catalogKey = (items: Procedure[]) => JSON.stringify(items.map((item) => [item.id, item.name, item.durationMinutes,
  item.preparationMinutes, item.isRelaxing, item.minIntervalDays, item.defaultWeeklyCount, item.defaultSessionCount,
  item.defaultPrerequisiteId, item.room]))

export const useProcedureCatalog = (initialProcedures: Procedure[], initialRevision: number, plans: PatientPlan[], setPlans: Dispatch<SetStateAction<PatientPlan[]>>) => {
  const router = useRouter()
  const [procedureDrafts, setProcedureDrafts] = useState(initialProcedures)
  const procedureDraftsRef = useRef(initialProcedures)
  const catalogRevisionRef = useRef(initialRevision)
  const procedureDraftVersion = useRef(0)
  const procedureSaveQueue = useRef(Promise.resolve())
  const pendingSnapshot = useRef<Procedure[] | null>(null)
  const saveTimer = useRef<number | null>(null)
  const hasUnsavedChanges = useRef(false)
  const procedureSaveFailed = useRef(false)
  const [procedureSaveError, setProcedureSaveError] = useState(false)
  const [catalogConflict, setCatalogConflict] = useState(false)
  const [procedureSaving, setProcedureSaving] = useState(false)

  useEffect(() => {
    const warnOnUnload = (event: BeforeUnloadEvent) => {
      if (!hasUnsavedChanges.current) return
      event.preventDefault()
      event.returnValue = ""
    }
    window.addEventListener("beforeunload", warnOnUnload)
    return () => window.removeEventListener("beforeunload", warnOnUnload)
  }, [])
  const setProcedureDraftsWithVersion: Dispatch<SetStateAction<Procedure[]>> = (next) => {
    procedureDraftVersion.current++
    const resolved = typeof next === "function" ? next(procedureDraftsRef.current) : next
    procedureDraftsRef.current = resolved
    setProcedureDrafts(resolved)
  }

  useEffect(() => {
    let active = true
    const version = procedureDraftVersion.current
    // Catalog data arrives from the server component. A server refresh must
    // not overwrite newer local changes that are still being saved.
    void procedureSaveQueue.current.then(() => {
      if (active && version === procedureDraftVersion.current && !procedureSaveFailed.current && !hasUnsavedChanges.current &&
        (initialRevision > catalogRevisionRef.current ||
          initialRevision === catalogRevisionRef.current && catalogKey(initialProcedures) === catalogKey(procedureDraftsRef.current))) {
        catalogRevisionRef.current = initialRevision
        procedureDraftsRef.current = initialProcedures
        setProcedureDrafts(initialProcedures)
      }
    })
    return () => { active = false }
  }, [initialProcedures, initialRevision])

  const reportSaved = useCallback(() => {
    procedureSaveFailed.current = false
    setProcedureSaveError(false)
    setCatalogConflict(false)
    try { window.localStorage.setItem(CATALOG_UPDATE_KEY, String(Date.now())) } catch { /* Storage is optional. */ }
  }, [])
  const enqueuePendingSave = useCallback(() => {
    if (saveTimer.current !== null) window.clearTimeout(saveTimer.current)
    saveTimer.current = null
    const nextProcedures = pendingSnapshot.current
    pendingSnapshot.current = null
    if (!nextProcedures) return
    procedureSaveQueue.current = procedureSaveQueue.current.then(async () => {
      try {
        await sendBackendRequest<Procedure[]>("/v1/procedures", "PUT", nextProcedures, {
          headers: {"If-Match": `"${catalogRevisionRef.current}"`},
          onResponse: (response) => {
            const revision = Number(response.headers.get("etag")?.replaceAll('"', ""))
            if (Number.isSafeInteger(revision)) catalogRevisionRef.current = revision
          },
        })
        reportSaved()
        if (catalogKey(nextProcedures) === catalogKey(procedureDraftsRef.current) && !pendingSnapshot.current) {
          hasUnsavedChanges.current = false
          setProcedureSaving(false)
        }
      } catch (error) {
        procedureSaveFailed.current = true
        setProcedureSaveError(true)
        if (error instanceof BackendRequestError && error.status === 409 && error.reason.includes("catalog revision changed")) setCatalogConflict(true)
        setProcedureSaving(false)
      }
    })
  }, [reportSaved])
  const persistProcedureDrafts = (nextProcedures: Procedure[], immediate = false) => {
    setProcedureDraftsWithVersion(nextProcedures)
    hasUnsavedChanges.current = true
    pendingSnapshot.current = nextProcedures
    setProcedureSaving(true)
    if (immediate) enqueuePendingSave()
    else {
      if (saveTimer.current !== null) window.clearTimeout(saveTimer.current)
      saveTimer.current = window.setTimeout(enqueuePendingSave, 350)
    }
  }
  const flushProcedureSaves = useCallback(async () => {
    while (pendingSnapshot.current || saveTimer.current !== null) {
      enqueuePendingSave()
      await procedureSaveQueue.current
    }
    await procedureSaveQueue.current
    return !procedureSaveFailed.current
  }, [enqueuePendingSave])

  useEffect(() => {
    const refreshCatalog = () => {
      const version = procedureDraftVersion.current
      void flushProcedureSaves().then((saved) => {
        if (saved && version === procedureDraftVersion.current) router.refresh()
      })
    }
    const handleVisibility = () => {
      if (document.visibilityState === "visible") refreshCatalog()
    }
    const handleStorage = (event: StorageEvent) => {
      if (event.key === CATALOG_UPDATE_KEY) refreshCatalog()
    }
    window.addEventListener("focus", refreshCatalog)
    document.addEventListener("visibilitychange", handleVisibility)
    window.addEventListener("storage", handleStorage)
    return () => {
      window.removeEventListener("focus", refreshCatalog)
      document.removeEventListener("visibilitychange", handleVisibility)
      window.removeEventListener("storage", handleStorage)
    }
  }, [router, flushProcedureSaves])
  const updateProcedure = (id: string, changes: Partial<Procedure>) => {
    if (changes.isRelaxing === false && procedureDraftsRef.current.some((procedure) => procedure.defaultPrerequisiteId === id)) {
      notify.warning("Расслабляющая процедура используется", "Сначала измените связанные процедуры в каталоге, затем снимите признак расслабляющей.")
      return
    }
    if (changes.defaultPrerequisiteId && !procedureDraftsRef.current.some((procedure) => procedure.id === changes.defaultPrerequisiteId && procedure.isRelaxing)) {
      notify.warning("Выберите расслабляющую процедуру", "Сначала отметьте подготовительную процедуру как расслабляющую.")
      return
    }
    persistProcedureDrafts(procedureDraftsRef.current.map((procedure) => procedure.id === id
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
    ...procedureDraftsRef.current,
    {id: crypto.randomUUID(), name: `Процедура ${procedureDraftsRef.current.length + 1}`, durationMinutes: 30,
      preparationMinutes: 20, isRelaxing: false, minIntervalDays: 1, defaultWeeklyCount: 1, defaultSessionCount: 1, defaultPrerequisiteId: "", room: ""},
  ], true)
  const removeProcedure = (id: string) => {
    enqueuePendingSave()
    const hadFailedSave = procedureSaveFailed.current
    const hadUnsavedBeforeRemoval = hasUnsavedChanges.current
    const removedIndex = procedureDraftsRef.current.findIndex((procedure) => procedure.id === id)
    if (removedIndex < 0) return
    const removed = procedureDraftsRef.current[removedIndex]
    setProcedureDraftsWithVersion((all) => all.filter((procedure) => procedure.id !== id))
    const removalVersion = procedureDraftVersion.current
    hasUnsavedChanges.current = true
    setProcedureSaving(true)
    procedureSaveQueue.current = procedureSaveQueue.current.then(async () => {
      // A failed full-catalog save leaves edits that DELETE cannot persist.
      // Keep those edits visible and unsaved instead of treating the deletion
      // as confirmation that the entire draft reached the server.
      if (procedureSaveFailed.current) {
        setProcedureDraftsWithVersion((all) => all.some((procedure) => procedure.id === id) ? all :
          [...all.slice(0, removedIndex), removed, ...all.slice(removedIndex)])
        setProcedureSaving(false)
        notify.warning("Каталог не сохранён", "Исправьте ошибку сохранения перед удалением процедуры.")
        return
      }
      try {
        await sendBackendRequest<null>("/v1/procedures/" + encodeURIComponent(id), "DELETE", undefined, {
          headers: {"If-Match": `"${catalogRevisionRef.current}"`},
          onResponse: (response) => {
            const revision = Number(response.headers.get("etag")?.replaceAll('"', ""))
            if (Number.isSafeInteger(revision)) catalogRevisionRef.current = revision
          },
        })
        setPlans((all) => all.map((plan) => ({
          ...plan,
          procedureIds: plan.procedureIds.filter((procedureId) => procedureId !== id),
          procedureCounts: Object.fromEntries(Object.entries(plan.procedureCounts).filter(([procedureId]) => procedureId !== id)),
          procedurePrerequisites: Object.fromEntries(Object.entries(plan.procedurePrerequisites ?? {}).filter(([targetId, prerequisiteId]) => targetId !== id && prerequisiteId !== id)),
          procedurePrerequisiteOverrides: Object.fromEntries(Object.entries(plan.procedurePrerequisiteOverrides ?? {}).filter(([targetId, prerequisiteId]) => targetId !== id && prerequisiteId !== id)),
        })))
        reportSaved()
        if (removalVersion === procedureDraftVersion.current && !pendingSnapshot.current) {
          hasUnsavedChanges.current = false
          setProcedureSaving(false)
        }
      } catch (error) {
        procedureSaveFailed.current = true
        setProcedureSaving(false)
        const editedDuringRemoval = removalVersion !== procedureDraftVersion.current
        setProcedureDraftsWithVersion((all) => all.some((procedure) => procedure.id === id) ? all :
          [...all.slice(0, removedIndex), removed, ...all.slice(removedIndex)])
        if (removalVersion === procedureDraftVersion.current - 1 && !pendingSnapshot.current) {
          hasUnsavedChanges.current = hadUnsavedBeforeRemoval && hadFailedSave
        }
        setProcedureSaveError(true)
        const conflict = error instanceof BackendRequestError && error.status === 409 && error.reason.includes("catalog revision changed")
        if (conflict) setCatalogConflict(true)
        // Edits queued during optimistic removal omitted the restored row.
        // Acknowledge the restored final snapshot after those requests finish.
        else if (editedDuringRemoval) persistProcedureDrafts(procedureDraftsRef.current, true)
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
  const retryProcedureSave = () => {
    if (catalogConflict) return
    if (!validProcedures) {
      notify.warning("Проверьте каталог", "Исправьте незаполненные или неверные поля, затем повторите сохранение.")
      return
    }
    persistProcedureDrafts(procedureDraftsRef.current, true)
  }
  const saveProcedureDrafts = async (nextProcedures: Procedure[]) => {
    persistProcedureDrafts(nextProcedures, true)
    return flushProcedureSaves()
  }
  useEffect(() => {
    if (!validProcedures) notify.warning("Проверьте параметры процедур", "Длительность: 1–420 мин, перерыв: 0–364 дня, максимум в неделю: 1–7, сеансов по умолчанию: 1–365, кабинет: не более 80 символов.")
  }, [validProcedures])

  return {procedureDrafts, setProcedureDraftsWithVersion, procedureSaveQueue, procedureSaveFailed, procedureSaveError, procedureSaving, catalogConflict,
    validProcedures, updateProcedure, addProcedure, removeProcedure, flushProcedureSaves, retryProcedureSave, saveProcedureDrafts}
}
