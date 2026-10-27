const DAY_MS = 86_400_000

export const parseDateKey = (dateKey: string): Date | undefined => {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(dateKey)) return undefined
  const [year, month, day] = dateKey.split("-").map(Number)
  const date = new Date(year, month - 1, day, 12)
  return date.getFullYear() === year && date.getMonth() === month - 1 && date.getDate() === day ? date : undefined
}

export const formatDateKey = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

export const addCalendarDays = (dateKey: string, count: number): string => {
  const date = parseDateKey(dateKey)
  if (!date) return ""
  date.setDate(date.getDate() + count)
  return formatDateKey(date)
}

export const inclusiveDayCount = (arrivalDate: string, departureDate: string): number => {
  const arrival = parseDateKey(arrivalDate)
  const departure = parseDateKey(departureDate)
  if (!arrival || !departure) return 0
  const arrivalUtc = Date.UTC(arrival.getFullYear(), arrival.getMonth(), arrival.getDate())
  const departureUtc = Date.UTC(departure.getFullYear(), departure.getMonth(), departure.getDate())
  return Math.round((departureUtc - arrivalUtc) / DAY_MS) + 1
}

const dateFormatter = new Intl.DateTimeFormat("ru-RU", {day: "numeric", month: "long", year: "numeric", timeZone: "UTC"})

export const formatDateLabel = (dateKey: string): string => {
  const date = parseDateKey(dateKey)
  return date ? dateFormatter.format(new Date(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()))) : dateKey
}
