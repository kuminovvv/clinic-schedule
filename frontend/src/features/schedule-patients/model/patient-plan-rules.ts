import type {Procedure} from "@/entities/procedure"
import {addCalendarDays, inclusiveDayCount, parseDateKey} from "@/shared/lib/date-range"
import type {PatientPlan} from "./types"
import {frequencyAdvice, minimumDaysForSessions} from "./course-frequency"

export const initialProcedureSessionCount = (procedure: Procedure | undefined, stayDays: number): number =>
  Math.min(Math.max(1, stayDays), Math.max(1, procedure?.defaultSessionCount ?? 1))

export const requiredRelaxingSessions = (plan: PatientPlan): Record<string, number> => {
  const needed: Record<string, number> = {}
  for (const [targetId, relaxingId] of Object.entries(plan.procedurePrerequisites ?? {})) {
    needed[relaxingId] = (needed[relaxingId] ?? 0) + (plan.procedureCounts[targetId] ?? 0)
  }
  return needed
}

export const balanceRelaxingSessions = (plan: PatientPlan): PatientPlan => {
  const counts = {...plan.procedureCounts}
  for (const [id, required] of Object.entries(requiredRelaxingSessions(plan))) {
    if (counts[id] && required <= plan.stayDays && counts[id] < required) counts[id] = required
  }
  return {...plan, procedureCounts: counts}
}

/**
 * Apply catalog defaults to newly configured high-severity courses.
 * Explicit per-patient overrides take precedence over catalog defaults.
 * A required relaxing procedure is also selected automatically.
 */
export const applyCatalogPrerequisites = (plan: PatientPlan, procedures: Procedure[]): PatientPlan => {
  if (plan.severity !== "high") {
    return {...plan, procedurePrerequisites: {}}
  }
  const byID = new Map(procedures.map((procedure) => [procedure.id, procedure]))
  const procedureIds = [...plan.procedureIds]
  const procedureCounts = {...plan.procedureCounts}
  const effective: Record<string, string> = {}
  const overrides = plan.procedurePrerequisiteOverrides ?? {}
  for (const targetId of plan.procedureIds) {
    const target = byID.get(targetId)
    if (!target || target.isRelaxing) continue
    const hasOverride = Object.prototype.hasOwnProperty.call(overrides, targetId)
    const relaxingId = hasOverride
      ? overrides[targetId]
      : target.defaultPrerequisiteId
    if (!relaxingId) continue
    const relaxing = byID.get(relaxingId)
    if (!relaxing?.isRelaxing || relaxingId === targetId) continue
    effective[targetId] = relaxingId
    if (!procedureIds.includes(relaxingId)) {
      procedureIds.push(relaxingId)
      procedureCounts[relaxingId] = initialProcedureSessionCount(relaxing, plan.stayDays)
    }
  }
  return balanceRelaxingSessions({...plan, procedureIds, procedureCounts, procedurePrerequisites: effective})
}

export const getPatientPlanIssue = (plans: PatientPlan[], procedures: Procedure[], courseKey: string): string | null => {
  for (const plan of plans) {
    const label = `Пациент ${plan.patientNumber}`
    const days = inclusiveDayCount(plan.arrivalDate, plan.departureDate)
    if (!parseDateKey(plan.arrivalDate) || !parseDateKey(plan.departureDate) || days < 1 || days > 365 || days !== plan.stayDays) {
      return `${label}: выберите корректные даты поступления и выезда (от 1 до 365 дней).`
    }
    if (plan.arrivalDate < courseKey || plan.departureDate >= addCalendarDays(courseKey, 365)) {
      return `${label}: лечение должно проходить в течение 365 дней от даты плана ${courseKey}. Измените дату плана или даты пациента.`
    }
    for (const id of plan.procedureIds) {
      const count = plan.procedureCounts[id]
      const procedure = procedures.find((item) => item.id === id)
      if (!procedure) return `${label}: назначенная процедура удалена из каталога. Уберите её из списка.`
      if (!Number.isInteger(count) || count < 1 || count > 365) {
        return `${label}: для «${procedure.name}» укажите целое количество сеансов от 1 до 365.`
      }
    }
    const required = requiredRelaxingSessions(plan)
    for (const [targetId, relaxingId] of Object.entries(plan.procedurePrerequisites ?? {})) {
      const target = procedures.find((item) => item.id === targetId)
      const relaxing = procedures.find((item) => item.id === relaxingId)
      if (plan.severity !== "high" || !target || target.isRelaxing || !relaxing?.isRelaxing ||
        !plan.procedureIds.includes(targetId) || !plan.procedureIds.includes(relaxingId)) {
        return `${label}: измените зависимость процедур — сложная и расслабляющая должны быть назначены пациенту.`
      }
    }
    for (const [id, count] of Object.entries(required)) {
      const relaxing = procedures.find((item) => item.id === id)
      const assigned = plan.procedureCounts[id] ?? 0
      if (!relaxing) continue
      const minimumDays = minimumDaysForSessions(relaxing, count)
      if (minimumDays === null || minimumDays > plan.stayDays) {
        const targets = Object.entries(plan.procedurePrerequisites ?? {})
          .filter(([, prerequisiteId]) => prerequisiteId === id)
          .map(([targetId]) => {
            const target = procedures.find((item) => item.id === targetId)
            return `«${target?.name ?? "процедура"}» — ${plan.procedureCounts[targetId] ?? 0}`
          })
          .join(", ")
        const details = frequencyAdvice(relaxing, count, plan.stayDays)
        return `${label}: назначено ${targets}. Для подготовки нужно ${count} сеансов «${relaxing.name}», но их нельзя разместить. ${details ?? ""} Увеличьте срок в «Датах поступления и выезда» или сократите сложные сеансы. Изменение правил каталога согласуйте с врачом.`
      }
      if (assigned < count) {
        return `${label}: системе нужно ${count} подготовительных сеансов «${relaxing.name}», но пока назначено ${assigned}. Измените количество сложных сеансов или даты лечения. Подготовка пересчитается сама.`
      }
    }
    for (const id of plan.procedureIds) {
      const procedure = procedures.find((item) => item.id === id)
      if (!procedure) continue
      const count = plan.procedureCounts[id] ?? 0
      const advice = frequencyAdvice(procedure, count, plan.stayDays)
      if (advice) {
        return `${label}: «${procedure.name}» — запрошено ${count} сеансов, но такой курс нарушает правила частоты. ${advice} Уменьшите количество сеансов или увеличьте даты лечения. Изменять правила каталога можно только после согласования с врачом.`
      }
    }
  }
  return null
}
