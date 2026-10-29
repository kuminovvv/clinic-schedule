"use client"

import {useEffect, useRef} from "react"
import {useRouter} from "next/navigation"
import type {Procedure} from "@/entities/procedure"
import {BackendRequestError, sendBackendRequest} from "@/shared/api/backend-client"
import {formatUnscheduledDetails, type UnscheduledDetail} from "@/shared/api/schedule-feedback"
import {notify} from "@/shared/lib/notifications"
import type {PatientPlan} from "../model/types"

type SchedulePreview = {
  unscheduledCount: number
  unscheduledDetails?: UnscheduledDetail[]
  warnings: string[]
}

type UseScheduleGenerationProps = {
  plans: PatientPlan[]
  procedureDrafts: Procedure[]
  saveProcedureDrafts: (procedures: Procedure[]) => Promise<boolean>
  selectedPatientNumber: number | null
  dateKey: string
  validProcedures: boolean
  validPlans: boolean
  planIssue: string | null
  isGenerating: boolean
  setIsGenerating: (isGenerating: boolean) => void
  setSaveError: (hasError: boolean) => void
  planConflictRef: {current: boolean}
  setPlanConflict: (hasConflict: boolean) => void
  flushProcedureSaves: () => Promise<boolean>
  procedureSaveFailed: {current: boolean}
  planSaveQueue: {current: Promise<void>}
  planSaveTimerRef: {current: number | null}
  lastSavedPlan: {current: string}
  savePlanDraft: (plans: PatientPlan[], patientNumber: number | null) => Promise<void>
}

const publishScheduleWarnings = (warnings: string[], courseVersion: string) => {
  if (typeof window === "undefined") return
  window.dispatchEvent(new CustomEvent("clinic:schedule-warnings", {detail: {warnings, courseVersion}}))
}

export const useScheduleGeneration = (props: UseScheduleGenerationProps) => {
  const router = useRouter()
  const {
    plans, procedureDrafts, saveProcedureDrafts, selectedPatientNumber, dateKey,
    validProcedures, validPlans, planIssue, isGenerating, setIsGenerating, setSaveError,
    planConflictRef, setPlanConflict, flushProcedureSaves, procedureSaveFailed,
    planSaveQueue, planSaveTimerRef, lastSavedPlan, savePlanDraft,
  } = props
  const confirmationId = `course-confirm:${dateKey}`
  const courseVersion = JSON.stringify({dateKey, plans, procedureDrafts})
  const currentCourseKey = JSON.stringify({plans, procedureDrafts, selectedPatientNumber})
  const currentCourseKeyRef = useRef(currentCourseKey)

  useEffect(() => {
    currentCourseKeyRef.current = currentCourseKey
    notify.dismiss(confirmationId)
    return () => notify.dismiss(confirmationId)
  }, [confirmationId, currentCourseKey])

  const finishGeneration = async () => {
    setIsGenerating(true)
    try {
      await sendBackendRequest("/v1/schedules/generate", "POST", {date: dateKey})
      publishScheduleWarnings([], courseVersion)
      router.push("/?date=" + dateKey)
    } catch {
      // Shared API notifications explain why generation could not finish.
    } finally {
      setIsGenerating(false)
    }
  }

  const flushPlanSaves = async () => {
    if (planSaveTimerRef.current !== null) {
      window.clearTimeout(planSaveTimerRef.current)
      planSaveTimerRef.current = null
    }
    if (planConflictRef.current) return false
    if (planIssue) {
      notify.constraint("Сначала завершите назначение", planIssue)
      return false
    }
    try {
      await planSaveQueue.current
      if (JSON.stringify({plans, selectedPatientNumber}) !== lastSavedPlan.current) await savePlanDraft(plans, selectedPatientNumber)
      setSaveError(false)
      return true
    } catch (error) {
      setSaveError(true)
      if (error instanceof BackendRequestError && error.status === 409) {
        planConflictRef.current = true
        setPlanConflict(true)
        notify.error("Данные изменились в другом окне", "Автосохранение остановлено, чтобы не потерять изменения. Обновите страницу.")
      }
      return false
    }
  }

  const changeCourseDate = async (nextDate: string) => {
    const date = new Date(`${nextDate}T00:00:00Z`)
    if (!/^\d{4}-\d{2}-\d{2}$/.test(nextDate) || Number.isNaN(date.getTime()) || date.toISOString().slice(0, 10) !== nextDate || nextDate === dateKey || isGenerating) return
    try {
      if (!(await flushProcedureSaves()) || procedureSaveFailed.current) return
      if (!(await flushPlanSaves())) return
      router.push(`/schedule/patients?date=${encodeURIComponent(nextDate)}`)
    } catch (error) {
      setSaveError(true)
      if (error instanceof BackendRequestError && error.status === 409) {
        planConflictRef.current = true
        setPlanConflict(true)
        notify.error("Данные изменились в другом окне", "Автосохранение остановлено, чтобы не потерять изменения. Обновите страницу.")
      }
    }
  }

  const submit = async () => {
    if (!validPlans) {
      publishScheduleWarnings(planIssue ? [planIssue] : ["Проверьте назначения и количество сеансов."], courseVersion)
      notify.constraint("Что нужно изменить перед составлением курса", planIssue ?? "Проверьте назначения и количество сеансов.")
      return
    }
    if (!validProcedures || planConflictRef.current || isGenerating) return
    setIsGenerating(true)
    if (planSaveTimerRef.current !== null) {
      window.clearTimeout(planSaveTimerRef.current)
      planSaveTimerRef.current = null
    }
    const savedProcedures = procedureDrafts.map((procedure) => ({...procedure, name: procedure.name.trim()}))
    const savedCourseVersion = JSON.stringify({dateKey, plans, procedureDrafts: savedProcedures})
    try {
      if (!(await saveProcedureDrafts(savedProcedures))) {
        setSaveError(true)
        setIsGenerating(false)
        return
      }
      await planSaveQueue.current
      await savePlanDraft(plans, selectedPatientNumber)
      setSaveError(false)
    } catch (error) {
      setSaveError(true)
      if (error instanceof BackendRequestError && error.status === 409) {
        planConflictRef.current = true
        setPlanConflict(true)
      }
      setIsGenerating(false)
      return
    }
    if (!plans.some((plan) => plan.procedureIds.length)) {
      setIsGenerating(false)
      return
    }
    try {
      const preview = await sendBackendRequest<SchedulePreview>("/v1/schedules/generate", "POST", {date: dateKey, preview: true})
      if (preview && preview.unscheduledCount > 0) {
        const warning = formatUnscheduledDetails(preview.unscheduledDetails ?? [], preview.unscheduledCount)
        publishScheduleWarnings([warning], savedCourseVersion)
        notify.constraint("Не все сеансы помещаются в курс", warning)
        return
      }
      if (preview?.warnings?.length) {
        publishScheduleWarnings(preview.warnings, savedCourseVersion)
        const previewCourseKey = JSON.stringify({plans, procedureDrafts: savedProcedures, selectedPatientNumber})
        notify.confirmConstraint(confirmationId, "Проверьте ограничения курса", preview.warnings.join("\n"), () => {
          if (currentCourseKeyRef.current !== previewCourseKey) {
            notify.constraint("Назначения изменились", "Проверьте курс и нажмите «Составить курс процедур» ещё раз.")
            return
          }
          void finishGeneration()
        })
        return
      }
      publishScheduleWarnings([], savedCourseVersion)
      await sendBackendRequest("/v1/schedules/generate", "POST", {date: dateKey})
      router.push("/?date=" + dateKey)
    } catch {
      // The shared API interceptor explains why generation was blocked.
    } finally {
      setIsGenerating(false)
    }
  }

  return {changeCourseDate, flushPlanSaves, submit}
}
