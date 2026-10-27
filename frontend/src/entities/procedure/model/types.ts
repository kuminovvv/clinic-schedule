export type Procedure = {
  id: string
  name: string
  durationMinutes: number
  /** Total minutes of preparation/changeover around treatment. */
  preparationMinutes: number
  isRelaxing: boolean
  minIntervalDays: number
  defaultWeeklyCount: number
  /** Suggested number of sessions when the procedure is assigned to a patient. */
  defaultSessionCount: number
  /** Optional relaxing procedure required before every complex session. */
  defaultPrerequisiteId?: string
  /** Optional dedicated office; absent/empty means organization-wide. */
  room?: string
}
