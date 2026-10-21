"use client"

import { useRef, useState } from "react"
import { Tick01StrokeRoundedIcon as Check } from "@/shared/lib/icons"
import {
  CALENDAR_SETTINGS_CHANGE_EVENT,
  TIME_STEP_OPTIONS,
  type TimeStepMinutes,
} from "@/entities/calendar-settings"
import { sendBackendRequest } from "@/shared/api/backend-client"
import { Label } from "@/shared/ui/shadcn/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/shadcn/select"

const hourOptions = Array.from({ length: 24 }, (_, hour) => hour)

function formatHour(hour: number) {
  if (hour === 24) return "23:59"
  return String(hour).padStart(2, "0") + ":00"
}

type CalendarSettings = { timeStepMinutes: TimeStepMinutes; displayStartHour: number; displayEndHour: number }

export function CalendarSettingsPage({ initialSettings }: { initialSettings: CalendarSettings }) {
  const [settings, setSettings] = useState(initialSettings)
  const [saveError, setSaveError] = useState(false)
  const confirmedSettings = useRef(initialSettings)
  const saveQueue = useRef(Promise.resolve())
  const latestSave = useRef(0)
  const { timeStepMinutes: timeStep, displayStartHour: startHour, displayEndHour: endHour } = settings

  const updateSettings = (nextSettings: CalendarSettings) => {
    const saveId = ++latestSave.current
    setSettings(nextSettings)
    saveQueue.current = saveQueue.current.then(async () => {
      try {
        await sendBackendRequest("/v1/calendar-settings", "PUT", nextSettings)
        confirmedSettings.current = nextSettings
        window.dispatchEvent(new Event(CALENDAR_SETTINGS_CHANGE_EVENT))
        if (saveId === latestSave.current) {
          setSettings(nextSettings)
          setSaveError(false)
        }
      } catch {
        if (saveId === latestSave.current) {
          setSettings(confirmedSettings.current)
          setSaveError(true)
        }
      }
    })
  }

  const handleTimeStepChange = (value: string | null) => {
    if (!value) return
    const minutes = Number(value) as TimeStepMinutes
    void updateSettings({...settings, timeStepMinutes: minutes})
  }

  const handleStartHourChange = (value: string | null) => {
    if (!value) return
    const nextStart = Number(value)
    const nextEnd = nextStart >= endHour ? Math.min(nextStart + 1, 24) : endHour
    void updateSettings({...settings, displayStartHour: nextStart, displayEndHour: nextEnd})
  }

  const handleEndHourChange = (value: string | null) => {
    if (!value) return
    void updateSettings({...settings, displayEndHour: Number(value)})
  }

  return (
    <div className="min-h-0 flex-1 px-4 py-6 text-slate-900 sm:px-6 sm:py-10">
      <div className="container mx-auto max-w-4xl">
        <section>
          <p className="text-sm font-medium text-blue-600">Настройки</p>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight">Календарь</h1>
          <p className="mt-2 text-sm text-slate-500">Настройте поведение календарной сетки под свой рабочий процесс.</p>

          <div className="mt-8 grid gap-2 sm:grid-cols-[minmax(0,1fr)_220px] sm:items-center sm:gap-6">
            <div>
              <Label className="text-sm font-medium" htmlFor="time-step">Шаг прилипания</Label>
              <p className="mt-1 text-sm text-slate-500">На этот интервал округляется время при выборе, перетаскивании и изменении длительности записи.</p>
            </div>
            <Select onValueChange={handleTimeStepChange} value={String(timeStep)}>
              <SelectTrigger aria-label="Шаг прилипания" className="h-10 w-full rounded-lg" id="time-step">
                <SelectValue>{timeStep + " минут"}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {TIME_STEP_OPTIONS.map((minutes) => (
                  <SelectItem key={minutes} value={String(minutes)}>{minutes} минут</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="mt-6 grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="calendar-start-hour">Начало отображения</Label>
              <Select onValueChange={handleStartHourChange} value={String(startHour)}>
                <SelectTrigger aria-label="Начало отображения календаря" className="h-10 w-full rounded-lg" id="calendar-start-hour">
                  <SelectValue>{formatHour(startHour)}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {hourOptions.slice(0, 24).map((hour) => (
                    <SelectItem key={hour} value={String(hour)}>{formatHour(hour)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="calendar-end-hour">Конец отображения</Label>
              <Select onValueChange={handleEndHourChange} value={String(endHour)}>
                <SelectTrigger aria-label="Конец отображения календаря" className="h-10 w-full rounded-lg" id="calendar-end-hour">
                  <SelectValue>{formatHour(endHour)}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {Array.from({ length: 24 - startHour }, (_, index) => startHour + index + 1).map((hour) => (
                    <SelectItem key={hour} value={String(hour)}>{formatHour(hour)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <p className="text-sm text-slate-500 sm:col-span-2">Задаёт видимый диапазон календаря в 24-часовом формате.</p>
          </div>

          {saveError ? (
            <p className="mt-5 text-sm text-red-600" role="alert">Новое значение не сохранилось. Попробуйте выбрать его ещё раз.</p>
          ) : (
            <p className="mt-5 flex items-center gap-1.5 text-sm text-emerald-700" role="status">
              <Check className="size-4" /> Настройка сохраняется автоматически.
            </p>
          )}
        </section>
      </div>
    </div>
  )
}
