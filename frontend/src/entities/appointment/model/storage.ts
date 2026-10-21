import type { Appointment } from "./types"

export const APPOINTMENTS_STORAGE_KEY = "clinic-schedule:appointments"
const appointmentColors = new Set(["blue", "teal", "orange", "purple"])

export type AppointmentLoadResult = {
  appointments: Appointment[]
  hasError: boolean
}

function isAppointment(value: unknown): value is Appointment {
  if (!value || typeof value !== "object") return false

  const appointment = value as Record<string, unknown>
  const startAt = new Date(String(appointment.startAt))
  const endAt = new Date(String(appointment.endAt))

  return (
    typeof appointment.id === "string" &&
    appointment.id.trim().length > 0 &&
    typeof appointment.title === "string" &&
    appointment.title.trim().length > 0 &&
    typeof appointment.startAt === "string" &&
    typeof appointment.endAt === "string" &&
    !Number.isNaN(startAt.getTime()) &&
    !Number.isNaN(endAt.getTime()) &&
    endAt > startAt &&
    typeof appointment.description === "string" &&
    typeof appointment.color === "string" &&
    appointmentColors.has(appointment.color) &&
    (appointment.generatedByPlanner === undefined || typeof appointment.generatedByPlanner === "boolean")
  )
}

export function loadAppointments() {
  try {
    const storedAppointments = window.localStorage.getItem(APPOINTMENTS_STORAGE_KEY)
    if (!storedAppointments) return { appointments: [], hasError: false }

    const parsedAppointments: unknown = JSON.parse(storedAppointments)
    if (!Array.isArray(parsedAppointments)) return { appointments: [], hasError: true }

    const seenIds = new Set<string>()
    const appointments = parsedAppointments.filter((value) => {
      if (!isAppointment(value) || seenIds.has(value.id)) return false
      seenIds.add(value.id)
      return true
    })
    return {
      appointments,
      hasError: appointments.length !== parsedAppointments.length,
    }
  } catch {
    return { appointments: [], hasError: true }
  }
}

export function saveAppointments(appointments: Appointment[]) {
  try {
    window.localStorage.setItem(APPOINTMENTS_STORAGE_KEY, JSON.stringify(appointments))
    return true
  } catch {
    return false
  }
}
