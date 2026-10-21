import "server-only"

const getBackendBaseUrl = () => process.env.BACKEND_INTERNAL_URL ?? "http://127.0.0.1:8080"

export const isDateKey = (value: unknown): value is string => {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
  const date = new Date(`${value}T00:00:00.000Z`)
  return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value
}

export const getClinicDateKey = () => {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: process.env.CLINIC_TIMEZONE ?? "Asia/Omsk",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date())
  const part = (type: "year" | "month" | "day") => parts.find((item) => item.type === type)?.value ?? ""
  return `${part("year")}-${part("month")}-${part("day")}`
}

export const getServerData = async <T>(path: string): Promise<T> => {
  const response = await fetch(getBackendBaseUrl() + path, { cache: "no-store" })
  if (!response.ok) throw new Error("Backend request failed: " + response.status)
  return response.json() as Promise<T>
}
