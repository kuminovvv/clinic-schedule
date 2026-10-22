import type {TimeStepMinutes} from "./storage"

export type WorkBreak = {id: string; name: string; start: string; end: string}
export type WorkDay = {weekday: number; open: boolean; start: string; end: string; breaks: WorkBreak[]}
export type CalendarSettings = {
  timeStepMinutes: TimeStepMinutes
  displayStartHour: number
  displayEndHour: number
  workWeek: WorkDay[]
}

export const WEEKDAY_NAMES = ["Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"]

export const clockMinutes = (value: string) => {
  if (!/^([01]\d|2[0-3]):[0-5]\d$|^24:00$/.test(value)) return NaN
  const [hours, minutes] = value.split(":").map(Number)
  return hours * 60 + minutes
}

export const workDayForDate = (days: WorkDay[], date: Date): WorkDay | undefined =>
  days.find((day) => day.weekday === (date.getDay() || 7))

export const canScheduleAt = (days: WorkDay[], start: Date, end: Date) => {
  if (!(start < end) || start.toDateString() !== end.toDateString()) return false
  const day = workDayForDate(days, start)
  if (!day?.open) return false
  const from = start.getHours() * 60 + start.getMinutes()
  const to = end.getHours() * 60 + end.getMinutes()
  return from >= clockMinutes(day.start) && to <= clockMinutes(day.end) &&
    day.breaks.every((item) => from >= clockMinutes(item.end) || to <= clockMinutes(item.start))
}

export const validateWorkWeek = (week: WorkDay[]): string | null => {
  if (week.length !== 7 || new Set(week.map((day) => day.weekday)).size !== 7) return "В настройках должны быть все семь дней недели."
  for (const day of week) {
    const label = WEEKDAY_NAMES[day.weekday - 1]
    if (!label || !(clockMinutes(day.start) < clockMinutes(day.end)) || clockMinutes(day.start) === 1440) return `Проверьте время работы: ${label ?? "день недели"}.`
    if (day.breaks.length > 12) return `Слишком много перерывов: ${label}.`
    const ids = new Set<string>()
    const sorted = [...day.breaks].sort((first, second) => clockMinutes(first.start) - clockMinutes(second.start))
    for (const [index, item] of sorted.entries()) {
      if (!item.name.trim() || item.name.length > 80 || !item.id || item.id.length > 80 || ids.has(item.id)) return `Укажите название каждого перерыва: ${label}.`
      ids.add(item.id)
      if (!(clockMinutes(item.start) < clockMinutes(item.end)) || clockMinutes(item.start) < clockMinutes(day.start) || clockMinutes(item.end) > clockMinutes(day.end)) return `Перерыв в ${label.toLowerCase()} должен находиться внутри рабочих часов.`
      if (index && clockMinutes(sorted[index - 1].end) > clockMinutes(item.start)) return `В ${label.toLowerCase()} перерывы пересекаются.`
    }
  }
  return null
}
