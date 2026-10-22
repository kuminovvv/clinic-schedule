"use client"

import {useState} from "react"
import {type CalendarSettings, type WorkBreak, WEEKDAY_NAMES, clockMinutes} from "@/entities/calendar-settings"
import {useCalendarSettingsAutosave} from "@/shared/lib/hooks/use-calendar-settings-autosave"
import {usePageDrivenScroll} from "@/shared/lib/hooks/use-page-driven-scroll"
import {Button} from "@/shared/ui/shadcn/button"
import {Input} from "@/shared/ui/shadcn/input"
import {Label} from "@/shared/ui/shadcn/label"
import {Switch} from "@/shared/ui/shadcn/switch"
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from "@/shared/ui/shadcn/select"
import {TIME_STEP_OPTIONS} from "@/entities/calendar-settings"

const hh = (value: number) => `${String(value).padStart(2, "0")}:00`
const timeRange = (start: string, end: string) => `${start}–${end}`

export function CalendarSettingsPage({initialSettings}: {initialSettings: CalendarSettings}) {
  const {settings, update, status, error, retry} = useCalendarSettingsAutosave(initialSettings)
  const [selectedDay, setSelectedDay] = useState(1)
  const [showDisplay, setShowDisplay] = useState(false)
  const {trackRef, panelRef, viewportRef, contentRef, panelStyle} = usePageDrivenScroll({
    topOffset: 64,
    bottomGap: 12,
    heightMode: "fit-viewport",
  })
  const day = settings.workWeek.find((item) => item.weekday === selectedDay)

  const changeDay = (weekday: number, apply: (current: CalendarSettings["workWeek"][number]) => CalendarSettings["workWeek"][number]) => {
    update((previous) => ({...previous, workWeek: previous.workWeek.map((item) => item.weekday === weekday ? apply(item) : item)}))
  }
  const changeBreak = (id: string, changes: Partial<WorkBreak>) => changeDay(selectedDay, (current) => ({
    ...current, breaks: current.breaks.map((item) => item.id === id ? {...item, ...changes} : item),
  }))
  const addBreak = () => {
    if (!day || day.breaks.length >= 12) return
    const from = clockMinutes(day.start)
    const to = clockMinutes(day.end)
    let begin = from
    for (; begin + 30 <= to; begin += 15) {
      if (day.breaks.every((item) => begin >= clockMinutes(item.end) || begin + 30 <= clockMinutes(item.start))) break
    }
    if (begin + 30 > to) return
    const clock = (minutes: number) => `${String(Math.floor(minutes / 60)).padStart(2, "0")}:${String(minutes % 60).padStart(2, "0")}`
    changeDay(selectedDay, (current) => ({...current, breaks: [...current.breaks, {id: crypto.randomUUID(), name: "Перерыв", start: clock(begin), end: clock(begin + 30)}]}))
  }
  const opened = settings.workWeek.filter((item) => item.open)
  const earliest = opened.length ? Math.min(...opened.map((item) => clockMinutes(item.start))) : NaN
  const latest = opened.length ? Math.max(...opened.map((item) => clockMinutes(item.end))) : NaN
  const hiddenHours = opened.length > 0 && (earliest < settings.displayStartHour * 60 || latest > settings.displayEndHour * 60)

  return (
    <div className="mx-auto w-full max-w-5xl px-3 py-4 sm:px-5" ref={trackRef}>
      <section className="sticky flex min-h-0 flex-col overflow-hidden rounded-2xl bg-white" ref={panelRef} style={panelStyle}>
        <header className="shrink-0 border-b border-slate-100 px-4 py-4 sm:px-6">
          <h1 className="text-xl font-semibold tracking-tight text-slate-900">График работы</h1>
          <p className="mt-1 text-sm text-slate-500">Выберите день и настройте часы приёма. Изменения сохраняются автоматически.</p>
        </header>

        <div className="min-h-0 flex-1 overflow-hidden" ref={viewportRef}>
          <div className="space-y-5 pb-5" ref={contentRef}>
            <div className="grid gap-4 px-4 pt-4 lg:grid-cols-[260px_minmax(0,1fr)] sm:px-6">
              <Label className="grid gap-1.5 text-sm lg:hidden">Выберите день недели
                <Select value={String(selectedDay)} onValueChange={(value) => value && setSelectedDay(Number(value))}>
                  <SelectTrigger className="h-11 w-full"><SelectValue>{WEEKDAY_NAMES[selectedDay - 1]}</SelectValue></SelectTrigger>
                  <SelectContent>{WEEKDAY_NAMES.map((name, index) => <SelectItem key={name} value={String(index + 1)}>{name}</SelectItem>)}</SelectContent>
                </Select>
              </Label>
              <nav aria-label="Дни недели" className="hidden space-y-1 lg:block">
                <h2 className="mb-2 text-sm font-semibold text-slate-900">Рабочая неделя</h2>
                {settings.workWeek.map((item) => (
                  <div className={`flex items-center gap-2 rounded-xl px-2 py-1 ${selectedDay === item.weekday ? "bg-blue-50" : ""}`} key={item.weekday}>
                    <Button
                      aria-pressed={selectedDay === item.weekday}
                      className="h-auto min-w-0 flex-1 justify-start whitespace-normal rounded-lg px-2 py-2 text-left"
                      onClick={() => setSelectedDay(item.weekday)}
                      size="sm"
                      type="button"
                      variant="ghost"
                    >
                      <span className="flex w-full flex-col gap-0.5">
                        <span className="font-medium">{WEEKDAY_NAMES[item.weekday - 1]}</span>
                        <span className="text-xs font-normal text-slate-500">{item.open ? `${timeRange(item.start, item.end)} · ${item.breaks.length === 0 ? "без перерывов" : `${item.breaks.length} перерыв(ов)`}` : "Выходной"}</span>
                      </span>
                    </Button>
                  </div>
                ))}
              </nav>

              <section aria-label="Настройка выбранного дня" className="min-w-0 rounded-2xl border border-slate-200 p-4 sm:p-5">
                {day && (
                  <>
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <h2 className="text-base font-semibold">{WEEKDAY_NAMES[selectedDay - 1]}</h2>
                      <Label className="flex items-center gap-2 text-sm text-slate-700">
                        <Switch aria-label={`${WEEKDAY_NAMES[selectedDay - 1]} — рабочий день`} checked={day.open} onCheckedChange={(open) => changeDay(selectedDay, (current) => ({...current, open}))}/>
                        {day.open ? "Рабочий день" : "Выходной"}
                      </Label>
                    </div>
                    {day.open ? (
                      <div className="mt-5 space-y-5">
                        <div>
                          <h3 className="mb-3 text-sm font-medium">Часы работы</h3>
                          <div className="grid max-w-sm grid-cols-2 gap-3">
                            <Label className="grid gap-1.5 text-sm">Начало
                              <Input aria-label={`${WEEKDAY_NAMES[selectedDay - 1]}: начало работы`} className="h-11 text-base" type="time" value={day.start} onChange={(event) => changeDay(selectedDay, (current) => ({...current, start: event.target.value}))}/>
                            </Label>
                            <Label className="grid gap-1.5 text-sm">Конец
                              <Input aria-label={`${WEEKDAY_NAMES[selectedDay - 1]}: конец работы`} className="h-11 text-base" type="time" value={day.end} onChange={(event) => changeDay(selectedDay, (current) => ({...current, end: event.target.value}))}/>
                            </Label>
                          </div>
                        </div>
                        <div className="border-t border-slate-100 pt-4">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <div>
                              <h3 className="text-sm font-medium">Перерывы</h3>
                              <p className="mt-0.5 text-xs text-slate-500">Обед, уборка, кварцевание — каждую неделю в выбранный день.</p>
                            </div>
                            <Button onClick={addBreak} disabled={day.breaks.length >= 12} size="sm" type="button" variant="outline">Добавить</Button>
                          </div>
                          {day.breaks.length === 0 && <p className="mt-4 text-sm text-slate-500">Перерывов нет.</p>}
                          <div className="mt-3 space-y-3">
                            {day.breaks.map((item) => (
                              <div className="grid gap-2 rounded-xl bg-slate-50 p-3 sm:grid-cols-[minmax(0,1fr)_7rem_7rem_auto] sm:items-end" key={item.id}>
                                <Label className="grid gap-1 text-xs text-slate-600">Название
                                  <Input aria-label="Название перерыва" className="h-10 bg-white" maxLength={80} value={item.name} onChange={(event) => changeBreak(item.id, {name: event.target.value})}/>
                                </Label>
                                <Label className="grid gap-1 text-xs text-slate-600">Начало
                                  <Input aria-label={`${item.name}: начало`} className="h-10 bg-white" type="time" value={item.start} onChange={(event) => changeBreak(item.id, {start: event.target.value})}/>
                                </Label>
                                <Label className="grid gap-1 text-xs text-slate-600">Конец
                                  <Input aria-label={`${item.name}: конец`} className="h-10 bg-white" type="time" value={item.end} onChange={(event) => changeBreak(item.id, {end: event.target.value})}/>
                                </Label>
                                <Button aria-label={`Удалить перерыв ${item.name}`} className="h-10" onClick={() => changeDay(selectedDay, (current) => ({...current, breaks: current.breaks.filter((entry) => entry.id !== item.id)}))} size="sm" type="button" variant="ghost">Убрать</Button>
                              </div>
                            ))}
                          </div>
                        </div>
                      </div>
                    ) : (
                      <div className="mt-5 rounded-xl bg-slate-50 p-4 text-sm text-slate-600">В этот день записи не создаются. Включите переключатель рядом с днём недели, чтобы задать часы работы.</div>
                    )}
                  </>
                )}
              </section>
            </div>

            <div className="border-t border-slate-100 px-4 pt-4 sm:px-6">
              <Button aria-expanded={showDisplay} className="h-10 px-3" onClick={() => setShowDisplay((current) => !current)} type="button" variant="outline">
                {showDisplay ? "Скрыть параметры отображения" : "Параметры отображения календаря"}
              </Button>
              {showDisplay && (
                <section className="mt-3 rounded-xl border border-slate-200 p-4" aria-label="Параметры отображения календаря">
                  <p className="mb-4 text-sm text-slate-500">Эти настройки меняют только видимую часть календаря, не расписание организации.</p>
                  {hiddenHours && <div className="mb-4 flex flex-wrap items-center justify-between gap-2 rounded-lg bg-amber-50 p-3 text-sm text-amber-900">
                    <span>Не все рабочие часы сейчас видны.</span>
                    <Button className="bg-white" onClick={() => update((current) => ({...current, displayStartHour: Math.floor(earliest / 60), displayEndHour: Math.ceil(latest / 60)}))} size="sm" type="button" variant="outline">Показать весь рабочий день</Button>
                  </div>}
                  <div className="grid gap-4 sm:grid-cols-3">
                    <Label className="grid gap-1.5 text-sm">Шаг выбора времени
                      <Select value={String(settings.timeStepMinutes)} onValueChange={(value) => value && update((current) => ({...current, timeStepMinutes: Number(value) as CalendarSettings["timeStepMinutes"]}))}>
                        <SelectTrigger className="h-11 w-full"><SelectValue>{settings.timeStepMinutes} мин</SelectValue></SelectTrigger>
                        <SelectContent>{TIME_STEP_OPTIONS.map((item) => <SelectItem key={item} value={String(item)}>{item} минут</SelectItem>)}</SelectContent>
                      </Select>
                    </Label>
                    <Label className="grid gap-1.5 text-sm">Показывать с
                      <Select value={String(settings.displayStartHour)} onValueChange={(value) => value && update((current) => ({...current, displayStartHour: Number(value), displayEndHour: Math.max(current.displayEndHour, Number(value) + 1)}))}>
                        <SelectTrigger className="h-11 w-full"><SelectValue>{hh(settings.displayStartHour)}</SelectValue></SelectTrigger>
                        <SelectContent>{Array.from({length: 24}, (_, i) => i).map((hour) => <SelectItem key={hour} value={String(hour)}>{hh(hour)}</SelectItem>)}</SelectContent>
                      </Select>
                    </Label>
                    <Label className="grid gap-1.5 text-sm">Показывать до
                      <Select value={String(settings.displayEndHour)} onValueChange={(value) => value && update((current) => ({...current, displayEndHour: Number(value)}))}>
                        <SelectTrigger className="h-11 w-full"><SelectValue>{hh(settings.displayEndHour)}</SelectValue></SelectTrigger>
                        <SelectContent>{Array.from({length: 24 - settings.displayStartHour}, (_, i) => i + settings.displayStartHour + 1).map((hour) => <SelectItem key={hour} value={String(hour)}>{hh(hour)}</SelectItem>)}</SelectContent>
                      </Select>
                    </Label>
                  </div>
                </section>
              )}
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 px-4 pt-4 text-sm sm:px-6" role="status" aria-live="polite">
              <div>{error ? <span className="text-red-700">{error}</span> : status === "saving" ? "Сохраняем изменения…" : status === "error" ? <span className="text-red-700">Не удалось сохранить изменения.</span> : status === "pending" ? "Изменения будут сохранены автоматически…" : "Все изменения сохранены."}</div>
              {status === "error" && <Button onClick={retry} size="sm" type="button" variant="outline">Повторить сохранение</Button>}
              <p className="w-full text-xs text-slate-500">Новые настройки действуют для будущих записей. Существующие записи не перемещаются.</p>
            </div>
          </div>
        </div>
      </section>
    </div>
  )
}
