import { Calendar } from "./calendar"
import { getServerData } from "@/shared/api/backend"
import type { Appointment } from "@/entities/appointment"
import type { TimeStepMinutes } from "@/entities/calendar-settings"

type CalendarSettings = { timeStepMinutes: TimeStepMinutes; displayStartHour: number; displayEndHour: number }
const shiftDateKey = (dateKey: string, offset: number) => {
  const [year, month, day] = dateKey.split("-").map(Number)
  const date = new Date(Date.UTC(year, month - 1, day + offset))
  return [date.getUTCFullYear(), String(date.getUTCMonth() + 1).padStart(2, "0"), String(date.getUTCDate()).padStart(2, "0")].join("-")
}

export const CalendarPage = async ({ dateKey }: { dateKey: string }) => {
  const parsedDate = dateKey
  const [year, month, day] = parsedDate.split("-").map(Number)
  const date = new Date(Date.UTC(year, month - 1, day))
  const weekStart = shiftDateKey(parsedDate, -((date.getUTCDay() + 6) % 7))
  const weekEnd = shiftDateKey(weekStart, 7)
  const [appointments, settings] = await Promise.all([
    getServerData<Appointment[]>(`/v1/appointments?from=${weekStart}&to=${weekEnd}`),
    getServerData<CalendarSettings>("/v1/calendar-settings"),
  ])
  return <Calendar initialAppointments={appointments} initialDateKey={parsedDate} initialSettings={settings} />
}
