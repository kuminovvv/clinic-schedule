export const CALENDAR_START_HOUR = 8
export const CALENDAR_END_HOUR = 18
export const WORKDAY_START_HOUR = 9
export const WORKDAY_END_HOUR = 16
export const LUNCH_START_HOUR = 13
export const LUNCH_END_HOUR = 14
export const HOUR_HEIGHT = 76

export function startOfWeek(date: Date) {
  const day = new Date(date)
  day.setHours(0, 0, 0, 0)
  day.setDate(day.getDate() - ((day.getDay() + 6) % 7))
  return day
}

export function addDays(date: Date, days: number) {
  const nextDate = new Date(date)
  nextDate.setDate(nextDate.getDate() + days)
  return nextDate
}

export function formatDateKey(date: Date) {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

export function formatDateTimeLocal(date: Date) {
  const hours = String(date.getHours()).padStart(2, "0")
  const minutes = String(date.getMinutes()).padStart(2, "0")
  return `${formatDateKey(date)}T${hours}:${minutes}`
}

export function getWeekDays(date: Date) {
  const monday = startOfWeek(date)
  return Array.from({ length: 7 }, (_, index) => addDays(monday, index))
}

export function formatWeekLabel(date: Date) {
  const days = getWeekDays(date)
  const monthFormatter = new Intl.DateTimeFormat("ru-RU", { month: "long" })
  const firstMonth = monthFormatter.format(days[0])
  const lastMonth = monthFormatter.format(days[6])
  const year = days[6].getFullYear()

  if (days[0].getFullYear() !== year) {
    return `${firstMonth} ${days[0].getFullYear()} — ${lastMonth} ${year}`
  }

  return firstMonth === lastMonth
    ? `${firstMonth} ${year}`
    : `${firstMonth} — ${lastMonth} ${year}`
}
