"use client"

import {useCallback, useEffect, useRef, useState} from "react"
import type {Procedure} from "@/entities/procedure"
import {BackendRequestError, sendBackendRequest} from "@/shared/api/backend-client"
import {notify} from "@/shared/lib/notifications"
import type {PatientPlan, SavedDayPlan, Severity} from "../model/types"
import {addCalendarDays, inclusiveDayCount} from "@/shared/lib/date-range"
import {useScheduleGeneration} from "./use-schedule-generation"
import {useProcedureCatalog} from "./use-procedure-catalog"
import {applyCatalogPrerequisites, getPatientPlanIssue, initialProcedureSessionCount} from "../model/patient-plan-rules"

const planContentKey = (plans: PatientPlan[], selectedPatientNumber: number | null) =>
    JSON.stringify({plans, selectedPatientNumber})

export const useSchedulePatients = ({procedures, initialPlan, dateKey}: {procedures: Procedure[]; initialPlan: SavedDayPlan; dateKey: string}) => {
    const [plans, setPlans] = useState<PatientPlan[]>(initialPlan.plans)
    const {procedureDrafts, setProcedureDraftsWithVersion, procedureSaveQueue, procedureSaveFailed, procedureSaveError,
        validProcedures, updateProcedure, addProcedure, removeProcedure, flushProcedureSaves} = useProcedureCatalog(procedures, plans, setPlans)
    const [selectedPatientNumber, setSelectedPatientNumber] = useState<number | null>(initialPlan.selectedPatientNumber)
    const [saveError, setSaveError] = useState(false)
    const [planConflict, setPlanConflict] = useState(false)
    const [isGenerating, setIsGenerating] = useState(false)
    const planRevision = useRef(initialPlan.revision)
    const lastSavedPlan = useRef(planContentKey(initialPlan.plans, initialPlan.selectedPatientNumber))
    const planSaveQueue = useRef(Promise.resolve())
    const planSaveTimer = useRef<number | null>(null)
    const planConflictRef = useRef(false)
    const planIssue = getPatientPlanIssue(plans, procedureDrafts, dateKey)
    const previousIssueRef = useRef(planIssue)

    useEffect(() => {
        const previousIssue = previousIssueRef.current
        previousIssueRef.current = planIssue
        if (!planIssue || planIssue === previousIssue) return
        // Wait for rapid changes in number inputs to settle before notifying.
        const timer = window.setTimeout(() => notify.constraint("Проверьте назначения пациента", planIssue), 650)
        return () => window.clearTimeout(timer)
    }, [planIssue])

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
        if (planContentKey(plans, selectedPatientNumber) === lastSavedPlan.current || planConflictRef.current || planIssue) return
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
                        notify.error("Данные изменились в другом окне", "Автосохранение остановлено, чтобы не потерять изменения. Обновите страницу.")
                    }
                }
            })
        }, 300)
        return () => {
            if (planSaveTimer.current !== null) window.clearTimeout(planSaveTimer.current)
        }
    }, [plans, selectedPatientNumber, savePlanDraft, planIssue])

    const addPatient = () => {
        const patientNumber = Math.max(0, ...plans.map((item) => item.patientNumber)) + 1
        setSelectedPatientNumber(patientNumber)
        setPlans((all) => [...all, {patientNumber, severity: "medium", arrivalDate: dateKey, departureDate: addCalendarDays(dateKey, 6), stayDays: 7, procedureIds: [], procedureCounts: {}, procedurePrerequisites: {}}])
    }
    const removePatient = (patientNumber: number) => {
        const remaining = plans.filter((plan) => plan.patientNumber !== patientNumber)
        setPlans(remaining)
        if (selectedPatientNumber === patientNumber) setSelectedPatientNumber(remaining[0]?.patientNumber ?? null)
    }
    const changeSeverity = (patientNumber: number, severity: Severity) =>
        setPlans((all) => all.map((plan) => plan.patientNumber === patientNumber
            ? applyCatalogPrerequisites({...plan, severity, procedurePrerequisiteOverrides: severity === "high" ? plan.procedurePrerequisiteOverrides : {}}, procedureDrafts)
            : plan))
    const changeTreatmentRange = (patientNumber: number, arrivalDate: string, departureDate: string) => {
        const stayDays = inclusiveDayCount(arrivalDate, departureDate)
        if (!Number.isInteger(stayDays) || stayDays < 1 || stayDays > 365) return
        setPlans((all) => all.map((plan) => {
            if (plan.patientNumber !== patientNumber) return plan
            // Changing dates must not silently replace explicitly assigned
            // sessions with new catalog defaults.
            return applyCatalogPrerequisites({...plan, arrivalDate, departureDate, stayDays}, procedureDrafts)
        }))
    }
    const changeProcedureCount = (patientNumber: number, procedureId: string, count: number) => {
        if (!Number.isInteger(count) || count < 1 || count > 365) return
        setPlans((all) => all.map((plan) => plan.patientNumber === patientNumber
            ? applyCatalogPrerequisites({...plan, procedureCounts: {...plan.procedureCounts, [procedureId]: count}}, procedureDrafts) : plan))
    }
    const changeProcedurePrerequisite = (patientNumber: number, procedureId: string, prerequisiteId: string | null | "catalog") => {
        setPlans((all) => all.map((plan) => {
            if (plan.patientNumber !== patientNumber) return plan
            const procedurePrerequisites = {...plan.procedurePrerequisites}
            const procedurePrerequisiteOverrides = {...plan.procedurePrerequisiteOverrides}
            delete procedurePrerequisites[procedureId]
            if (prerequisiteId === "catalog") {
                delete procedurePrerequisiteOverrides[procedureId]
            } else {
                procedurePrerequisiteOverrides[procedureId] = prerequisiteId
            }
            return applyCatalogPrerequisites({...plan, procedurePrerequisites, procedurePrerequisiteOverrides}, procedureDrafts)
        }))
    }
    const toggleProcedure = (procedureId: string, checked: boolean) => {
        setPlans((all) => all.map((plan) => {
        if (plan.patientNumber !== selectedPatientNumber) return plan
        const procedureIds = checked ? [...plan.procedureIds, procedureId] : plan.procedureIds.filter((id) => id !== procedureId)
        const procedureCounts = {...plan.procedureCounts}
        const removedLinkedTargets = !checked ? Object.entries(plan.procedurePrerequisites ?? {})
            .filter(([, prerequisiteId]) => prerequisiteId === procedureId)
            .map(([targetId]) => targetId) : []
        const procedurePrerequisites = Object.fromEntries(Object.entries(plan.procedurePrerequisites ?? {}).filter(([targetId, prerequisiteId]) => checked || targetId !== procedureId && prerequisiteId !== procedureId))
        const procedurePrerequisiteOverrides = {...plan.procedurePrerequisiteOverrides}
        if (!checked) {
            delete procedurePrerequisiteOverrides[procedureId]
            for (const targetId of removedLinkedTargets) procedurePrerequisiteOverrides[targetId] = null
        }
        if (checked) {
            const procedure = procedureDrafts.find((item) => item.id === procedureId)
            procedureCounts[procedureId] = initialProcedureSessionCount(procedure, plan.stayDays)
        } else delete procedureCounts[procedureId]
        return applyCatalogPrerequisites({...plan, procedureIds, procedureCounts, procedurePrerequisites, procedurePrerequisiteOverrides}, procedureDrafts)
        }))
    }
    const validPlans = planIssue === null
    const {changeCourseDate, flushPlanSaves, submit} = useScheduleGeneration({
        plans, procedureDrafts, setProcedureDrafts: setProcedureDraftsWithVersion, selectedPatientNumber, dateKey,
        validProcedures, validPlans, planIssue, isGenerating, setIsGenerating, setSaveError,
        planConflictRef, setPlanConflict, procedureSaveQueue, procedureSaveFailed,
        planSaveQueue, planSaveTimerRef: planSaveTimer, lastSavedPlan, savePlanDraft,
    })

    return {plans, procedureDrafts, selectedPatientNumber, setSelectedPatientNumber, saveError: saveError || procedureSaveError, planIssue, planConflict, isGenerating, addPatient, removePatient, changeSeverity, changeTreatmentRange, changeProcedureCount, changeProcedurePrerequisite, toggleProcedure, updateProcedure, addProcedure, removeProcedure, validProcedures, validPlans, flushProcedureSaves, flushPlanSaves, changeCourseDate, submit}
}
