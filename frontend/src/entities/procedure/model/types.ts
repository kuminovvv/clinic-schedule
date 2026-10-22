export type Procedure = {
  id: string
  name: string
  durationMinutes: number
  /** Total minutes of preparation/changeover around treatment. */
  preparationMinutes: number
  isRelaxing: boolean
  minIntervalDays: number
  defaultWeeklyCount: number
}
