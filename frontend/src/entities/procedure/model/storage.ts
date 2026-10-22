import type { Procedure } from "./types"

export const PROCEDURES_STORAGE_KEY = "clinic-schedule:procedures"

export function createDefaultProcedures(): Procedure[] {
  return Array.from({ length: 20 }, (_, index) => ({
    id: "procedure-" + (index + 1),
    name: "Процедура " + (index + 1),
    durationMinutes: 30,
    preparationMinutes: 20,
    isRelaxing: false,
    minIntervalDays: 1,
    defaultWeeklyCount: 1,
  }))
}

function isProcedure(value: unknown): value is Procedure {
  if (!value || typeof value !== "object") return false
  const procedure = value as Record<string, unknown>
  return typeof procedure.id === "string" && procedure.id.trim().length > 0 && typeof procedure.name === "string" &&
    procedure.name.trim().length > 0 && typeof procedure.durationMinutes === "number" &&
    Number.isInteger(procedure.durationMinutes) && procedure.durationMinutes > 0 && procedure.durationMinutes <= 420 &&
    (procedure.preparationMinutes === undefined || Number.isInteger(procedure.preparationMinutes) && (procedure.preparationMinutes as number) >= 0 && procedure.durationMinutes + (procedure.preparationMinutes as number) <= 420) &&
    typeof procedure.isRelaxing === "boolean" &&
    (procedure.minIntervalDays === undefined || Number.isInteger(procedure.minIntervalDays) && (procedure.minIntervalDays as number) >= 1 && (procedure.minIntervalDays as number) <= 365) &&
    (procedure.defaultWeeklyCount === undefined || Number.isInteger(procedure.defaultWeeklyCount) && (procedure.defaultWeeklyCount as number) >= 1 && (procedure.defaultWeeklyCount as number) <= 7)
}

export function loadProcedures() {
  try {
    const raw = window.localStorage.getItem(PROCEDURES_STORAGE_KEY)
    if (!raw) return { procedures: createDefaultProcedures(), hasError: false }
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed) || parsed.some((item) => !isProcedure(item)) || new Set(parsed.map((item) => (item as Procedure).id)).size !== parsed.length) {
      return { procedures: createDefaultProcedures(), hasError: true }
    }
    return { procedures: (parsed as Procedure[]).map((procedure) => {
      const legacy = procedure.preparationMinutes === undefined
      return {...procedure, durationMinutes: legacy && procedure.durationMinutes > 20 ? procedure.durationMinutes - 20 : procedure.durationMinutes, preparationMinutes: legacy && procedure.durationMinutes > 20 ? 20 : 0, minIntervalDays: procedure.minIntervalDays ?? 1, defaultWeeklyCount: procedure.defaultWeeklyCount ?? 1}
    }), hasError: false }
  } catch {
    return { procedures: createDefaultProcedures(), hasError: true }
  }
}

export function saveProcedures(procedures: Procedure[]) {
  try {
    window.localStorage.setItem(PROCEDURES_STORAGE_KEY, JSON.stringify(procedures))
    return true
  } catch {
    return false
  }
}
