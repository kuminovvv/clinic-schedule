export type Severity = "high" | "medium" | "low"
export type PatientPlan = {
  patientNumber: number
  severity: Severity
  arrivalDate: string
  departureDate: string
  /** Derived during the compatibility period; the date range is authoritative. */
  stayDays: number
  procedureIds: string[]
  procedureCounts: Record<string, number>
  procedurePrerequisites?: Record<string, string>
  /** Sparse per-patient override: null explicitly turns off catalog requirement. */
  procedurePrerequisiteOverrides?: Record<string, string | null>
}
export type SavedDayPlan = {
  date: string
  revision: number
  selectedPatientNumber: number | null
  plans: PatientPlan[]
}
