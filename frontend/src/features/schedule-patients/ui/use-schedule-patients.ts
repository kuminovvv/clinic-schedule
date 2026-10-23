"use client"

import {useCallback, useEffect, useRef, useState} from "react"
import {useRouter} from "next/navigation"
import type {Procedure} from "@/entities/procedure"
import {BackendRequestError, sendBackendRequest} from "@/shared/api/backend-client"
import {notify} from "@/shared/lib/notifications"
import type {PatientPlan, SavedDayPlan, Severity} from "../model/types"

type SchedulePreview = {
    scheduledCount: number
    unscheduledCount: number
    unscheduledPatientNumbers: number[]
    warnings: string[]
}

const planContentKey = (plans: PatientPlan[], selectedPatientNumber: number | null) =>
    JSON.stringify({plans, selectedPatientNumber})

export const useSchedulePatients = ({procedures, initialPlan, dateKey}: {procedures: Procedure[]; initialPlan: SavedDayPlan; dateKey: string}) => {
    const router = useRouter()
    const [plans, setPlans] = useState<PatientPlan[]>(initialPlan.plans)
    const [procedureDrafts, setProcedureDrafts] = useState(procedures)
    const [selectedPatientNumber, setSelectedPatientNumber] = useState<number | null>(initialPlan.selectedPatientNumber)
    const [saveError, setSaveError] = useState(false)
    const [planConflict, setPlanConflict] = useState(false)
    const [isGenerating, setIsGenerating] = useState(false)
    const confirmationId = `course-confirm:${dateKey}`
    const currentCourseKey = JSON.stringify({plans, procedureDrafts, selectedPatientNumber})
    const currentCourseKeyRef = useRef(currentCourseKey)
    const procedureSaveQueue = useRef(Promise.resolve())
    const planRevision = useRef(initialPlan.revision)
    const lastSavedPlan = useRef(planContentKey(initialPlan.plans, initialPlan.selectedPatientNumber))
    const planSaveQueue = useRef(Promise.resolve())
    const planSaveTimer = useRef<number | null>(null)
    const planConflictRef = useRef(false)

    useEffect(() => {
        currentCourseKeyRef.current = currentCourseKey
        notify.dismiss(confirmationId)
        return () => notify.dismiss(confirmationId)
    }, [confirmationId, currentCourseKey])

    const savePlanDraft = useCallback(async (draftPlans: PatientPlan[], draftSelectedPatientNumber: number | null) => {
        const contentKey = planContentKey(draftPlans, draftSelectedPatientNumber)
        if (contentKey === lastSavedPlan.current || planConflictRef.current) return
        const saved = await sendBackendRequest<SavedDayPlan>("/v1/day-plans", "PUT", {
            date: dateKey,
            revision: planRevision.current,
            selectedPatientNumber: draftSelectedPatientNumber,
            plans: draftPlans,
        })
        if (saved) {
            planRevision.current = saved.revision
            lastSavedPlan.current = contentKey
        }
    }, [dateKey])

    useEffect(() => {
        if (planContentKey(plans, selectedPatientNumber) === lastSavedPlan.current || planConflictRef.current) return
        planSaveTimer.current = window.setTimeout(() => {
            planSaveTimer.current = null
            planSaveQueue.current = planSaveQueue.current.then(async () => {
                if (planConflictRef.current) return
                try {
                    await savePlanDraft(plans, selectedPatientNumber)
                    setSaveError(false)
                } catch (error) {
                    setSaveError(true)
                    if (error instanceof BackendRequestError && error.status === 409) {
                        planConflictRef.current = true
                        setPlanConflict(true)
                    }
                }
            })
        }, 300)
        return () => {
            if (planSaveTimer.current !== null) window.clearTimeout(planSaveTimer.current)
        }
    }, [plans, selectedPatientNumber, savePlanDraft])

    const addPatient = () => {
        const patientNumber = Math.max(0, ...plans.map((item) => item.patientNumber)) + 1
        setSelectedPatientNumber(patientNumber)
        setPlans((all) => [...all, {patientNumber, severity: "medium", stayDays: 7, procedureIds: [], procedureCounts: {}}])
    }
    const removePatient = (patientNumber: number) => {
        const remaining = plans.filter((plan) => plan.patientNumber !== patientNumber)
        setPlans(remaining)
        if (selectedPatientNumber === patientNumber) setSelectedPatientNumber(remaining[0]?.patientNumber ?? null)
    }
    const changeSeverity = (patientNumber: number, severity: Severity) =>
        setPlans((all) => all.map((plan) => plan.patientNumber === patientNumber ? {...plan, severity} : plan))
    const changeStay = (patientNumber: number, stayDays: number) => {
        if (!Number.isInteger(stayDays) || stayDays < 1 || stayDays > 365) return
        setPlans((all) => all.map((plan) => {
            if (plan.patientNumber !== patientNumber) return plan
            const procedureCounts = {...plan.procedureCounts}
            for (const id of plan.procedureIds) {
                const procedure = procedureDrafts.find((item) => item.id === id)
                const weeklyCount = procedure?.defaultWeeklyCount ?? 1
                const suggestedBefore = Math.max(1, Math.ceil(plan.stayDays * weeklyCount / 7))
                if (procedureCounts[id] === suggestedBefore) procedureCounts[id] = Math.max(1, Math.ceil(stayDays * weeklyCount / 7))
            }
            return {...plan, stayDays, procedureCounts}
        }))
    }
    const changeProcedureCount = (patientNumber: number, procedureId: string, count: number) => {
        if (!Number.isInteger(count) || count < 1 || count > 365) return
        setPlans((all) => all.map((plan) => plan.patientNumber === patientNumber
            ? {...plan, procedureCounts: {...plan.procedureCounts, [procedureId]: count}} : plan))
    }
    const toggleProcedure = (procedureId: string, checked: boolean) => setPlans((all) => all.map((plan) => {
        if (plan.patientNumber !== selectedPatientNumber) return plan
        const procedureIds = checked ? [...plan.procedureIds, procedureId] : plan.procedureIds.filter((id) => id !== procedureId)
        const procedureCounts = {...plan.procedureCounts}
        if (checked) {
            const procedure = procedureDrafts.find((item) => item.id === procedureId)
            procedureCounts[procedureId] = Math.max(1, Math.ceil(plan.stayDays * (procedure?.defaultWeeklyCount ?? 1) / 7))
        } else delete procedureCounts[procedureId]
        return {...plan, procedureIds, procedureCounts}
    }))
    const persistProcedureDrafts = (nextProcedures: Procedure[]) => {
        setProcedureDrafts(nextProcedures)
        procedureSaveQueue.current = procedureSaveQueue.current.then(async () => {
            try {
                await sendBackendRequest<Procedure[]>("/v1/procedures", "PUT", nextProcedures)
                setSaveError(false)
            } catch {
                setSaveError(true)
            }
        })
    }
    const updateProcedure = (id: string, changes: Partial<Procedure>) =>
        persistProcedureDrafts(procedureDrafts.map((procedure) => procedure.id === id ? {...procedure, ...changes} : procedure))
    const addProcedure = () => persistProcedureDrafts([
        ...procedureDrafts,
        {
            id: crypto.randomUUID(),
            name: "Процедура " + (procedureDrafts.length + 1),
            durationMinutes: 30,
            preparationMinutes: 20,
            isRelaxing: false,
            minIntervalDays: 1,
            defaultWeeklyCount: 1,
            room: "",
        },
    ])
    const removeProcedure = (id: string) => {
        procedureSaveQueue.current = procedureSaveQueue.current.then(async () => {
            try {
                await sendBackendRequest<null>("/v1/procedures/" + encodeURIComponent(id), "DELETE")
                setProcedureDrafts((all) => all.filter((procedure) => procedure.id !== id))
                setPlans((all) => all.map((plan) => ({
                    ...plan,
                    procedureIds: plan.procedureIds.filter((procedureId) => procedureId !== id),
                    procedureCounts: Object.fromEntries(Object.entries(plan.procedureCounts).filter(([procedureId]) => procedureId !== id)),
                })))
                setSaveError(false)
            } catch (error) {
                if (!(error instanceof BackendRequestError && error.status === 409)) setSaveError(true)
            }
        })
    }
    const validProcedures = procedureDrafts.every((procedure) =>
        procedure.name.trim() && Number.isInteger(procedure.durationMinutes) && procedure.durationMinutes >= 1 && procedure.durationMinutes <= 420 &&
        Number.isInteger(procedure.preparationMinutes) && procedure.preparationMinutes >= 0 && procedure.durationMinutes + procedure.preparationMinutes <= 420 &&
        Number.isInteger(procedure.minIntervalDays) && procedure.minIntervalDays >= 1 && procedure.minIntervalDays <= 365 &&
        Number.isInteger(procedure.defaultWeeklyCount) && procedure.defaultWeeklyCount >= 1 && procedure.defaultWeeklyCount <= 7 &&
        (procedure.room === undefined || procedure.room.trim().length <= 80),
    )
    const validPlans = plans.every((plan) => Number.isInteger(plan.stayDays) && plan.stayDays >= 1 && plan.stayDays <= 365 &&
        plan.procedureIds.every((id) => Number.isInteger(plan.procedureCounts[id]) && plan.procedureCounts[id] >= 1 && plan.procedureCounts[id] <= plan.stayDays))
    const finishGeneration = async () => {
        setIsGenerating(true)
        try {
            await sendBackendRequest("/v1/schedules/generate", "POST", {date: dateKey})
            router.push("/?date=" + dateKey)
        } catch {
            // Shared API interceptor displays the reason.
        } finally {
            setIsGenerating(false)
        }
    }
    const submit = async () => {
        if (!validProcedures || !validPlans || planConflictRef.current || isGenerating) return
        setIsGenerating(true)
        if (planSaveTimer.current !== null) {
            window.clearTimeout(planSaveTimer.current)
            planSaveTimer.current = null
        }
        const savedProcedures = procedureDrafts.map((procedure) => ({...procedure, name: procedure.name.trim()}))
        setProcedureDrafts(savedProcedures)
        try {
            await procedureSaveQueue.current
            await sendBackendRequest<Procedure[]>("/v1/procedures", "PUT", savedProcedures)
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
                notify.constraint(
                    "Не все сеансы помещаются в курс",
                    `Не удалось разместить ${preview.unscheduledCount} сеансов. Пациенты: ${preview.unscheduledPatientNumbers.join(", ")}. Уменьшите число сеансов или измените интервалы и срок пребывания.`,
                )
                return
            }
            if (preview?.warnings?.length) {
                const previewCourseKey = JSON.stringify({plans, procedureDrafts: savedProcedures, selectedPatientNumber})
                notify.confirmConstraint(
                    confirmationId,
                    "Проверьте ограничения курса",
                    preview.warnings.join("\n"),
                    () => {
                        if (currentCourseKeyRef.current !== previewCourseKey) {
                            notify.constraint("Назначения изменились", "Проверьте курс и нажмите «Составить курс процедур» ещё раз.")
                            return
                        }
                        void finishGeneration()
                    },
                )
                return
            }
            await sendBackendRequest("/v1/schedules/generate", "POST", {date: dateKey})
            router.push("/?date=" + dateKey)
        } catch {
            // The shared API interceptor explains why generation was blocked.
        } finally {
            setIsGenerating(false)
        }
    }

    return {plans, procedureDrafts, selectedPatientNumber, setSelectedPatientNumber, saveError, planConflict, isGenerating, addPatient, removePatient, changeSeverity, changeStay, changeProcedureCount, toggleProcedure, updateProcedure, addProcedure, removeProcedure, validProcedures, validPlans, submit}
}
