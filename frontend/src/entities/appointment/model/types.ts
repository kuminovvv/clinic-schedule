export type AppointmentColor = "blue" | "teal" | "orange" | "purple"

export type Appointment = {
  id: string
  title: string
  startAt: string
  endAt: string
  description: string
  color: AppointmentColor
  status?: "scheduled" | "cancelled"
  generatedByPlanner?: boolean
  patientNumber?: number
  procedureId?: string
  sourcePlanDate?: string
  room?: string
}

export type AppointmentInput = Omit<Appointment, "id">
