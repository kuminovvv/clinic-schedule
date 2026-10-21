export type Severity = "high" | "medium" | "low"
export type PatientPlan = {
  patientNumber: number
  severity: Severity
  stayDays: number
  procedureIds: string[]
  procedureCounts: Record<string, number>
}
export type SavedDayPlan = {
  date: string
  revision: number
  selectedPatientNumber: number | null
  plans: PatientPlan[]
}
