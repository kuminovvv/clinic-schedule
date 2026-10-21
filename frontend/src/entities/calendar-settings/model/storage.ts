export const CALENDAR_SETTINGS_STORAGE_KEY = "clinic-schedule:calendar-settings"
export const CALENDAR_DISPLAY_RANGE_STORAGE_KEY = "clinic-schedule:calendar-display-range"
export const CALENDAR_SETTINGS_CHANGE_EVENT = "calendar-settings-change"
export const TIME_STEP_OPTIONS = [5, 10, 15, 30] as const
export type TimeStepMinutes = (typeof TIME_STEP_OPTIONS)[number]
export const DEFAULT_TIME_STEP_MINUTES: TimeStepMinutes = 10
export const DEFAULT_CALENDAR_DISPLAY_RANGE = 818

export function loadCalendarDisplayRange() {
  try {
    const storedValue = Number(window.localStorage.getItem(CALENDAR_DISPLAY_RANGE_STORAGE_KEY))
    const startHour = Math.floor(storedValue / 100)
    const endHour = storedValue % 100
    return Number.isInteger(storedValue) && startHour >= 0 && startHour < 24 && endHour > startHour && endHour <= 24
      ? storedValue
      : DEFAULT_CALENDAR_DISPLAY_RANGE
  } catch {
    return DEFAULT_CALENDAR_DISPLAY_RANGE
  }
}

export function saveCalendarDisplayRange(startHour: number, endHour: number) {
  const value = startHour * 100 + endHour
  if (!Number.isInteger(startHour) || !Number.isInteger(endHour) || startHour < 0 || startHour > 23 || endHour <= startHour || endHour > 24) {
    return false
  }
  try {
    window.localStorage.setItem(CALENDAR_DISPLAY_RANGE_STORAGE_KEY, String(value))
    window.dispatchEvent(new Event(CALENDAR_SETTINGS_CHANGE_EVENT))
    return true
  } catch {
    return false
  }
}

export function loadTimeStepMinutes(): TimeStepMinutes {
  try {
    const storedValue = Number(window.localStorage.getItem(CALENDAR_SETTINGS_STORAGE_KEY))
    return TIME_STEP_OPTIONS.includes(storedValue as TimeStepMinutes)
      ? storedValue as TimeStepMinutes
      : DEFAULT_TIME_STEP_MINUTES
  } catch {
    return DEFAULT_TIME_STEP_MINUTES
  }
}

export function saveTimeStepMinutes(minutes: TimeStepMinutes) {
  try {
    window.localStorage.setItem(CALENDAR_SETTINGS_STORAGE_KEY, String(minutes))
    window.dispatchEvent(new Event(CALENDAR_SETTINGS_CHANGE_EVENT))
    return true
  } catch {
    return false
  }
}
