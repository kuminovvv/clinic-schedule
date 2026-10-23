"use client"

import {useEffect, useRef, useState} from "react"
import {useRouter} from "next/navigation"
import {
    type Appointment,
    type AppointmentInput,
} from "@/entities/appointment"
import {CALENDAR_SETTINGS_CHANGE_EVENT, canScheduleAt, clockMinutes, workDayForDate, type CalendarSettings, type WorkDay} from "@/entities/calendar-settings"
import {AppointmentDialog} from "@/features/manage-appointment/ui/appointment-dialog"
import {
    addDays,
    formatDateKey,
    formatDateTimeLocal,
} from "@/shared/lib/calendar-date"
import {getClientData, sendBackendRequest} from "@/shared/api/backend-client"
import {notify} from "@/shared/lib/notifications"
import {Button} from "@/shared/ui/shadcn/button"
import {CalendarToolbar} from "./calendar-toolbar"
import {ClearScheduleDialog} from "./clear-schedule-dialog"
import {usePageDrivenScroll} from "@/shared/lib/hooks/use-page-driven-scroll"
import {WeekGrid} from "./week-grid"

type DialogState = {
    initialStartAt: string
    initialEndAt?: string
    appointment?: Appointment
    anchor?: DOMRect
}

function parseDateKey(value?: string) {
    if (!value || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return null
    const [year, month, day] = value.split("-").map(Number)
    const date = new Date(year, month - 1, day)
    return formatDateKey(date) === value ? date : null
}

function getDefaultStart(date: Date, currentTime: Date, week: WorkDay[]) {
    const start = new Date(date)
    const sameDay = start.toDateString() === currentTime.toDateString()
    if (sameDay) start.setTime(currentTime.getTime())
    start.setSeconds(0, 0)
    for (let dayOffset = 0; dayOffset < 14; dayOffset++) {
        const rule = workDayForDate(week, start)
        if (rule?.open) {
            const opening = clockMinutes(rule.start)
            const current = dayOffset === 0 && sameDay ? start.getHours() * 60 + start.getMinutes() : opening
            for (let minutes = Math.max(opening, Math.ceil(current / 10) * 10); minutes + 50 <= clockMinutes(rule.end); minutes += 10) {
                const candidate = new Date(start)
                candidate.setHours(0, minutes, 0, 0)
                if (canScheduleAt(week, candidate, new Date(candidate.getTime() + 50 * 60_000))) return candidate
            }
        }
        start.setDate(start.getDate() + 1)
        start.setHours(0, 0, 0, 0)
    }
    return null
}

export function Calendar({initialDateKey, initialAppointments, initialSettings}: {
    initialDateKey: string;
    initialAppointments: Appointment[];
    initialSettings: CalendarSettings
}) {
    const router = useRouter()
    const [selectedDate, setSelectedDate] = useState<Date>(() => parseDateKey(initialDateKey) ?? new Date())
    const [selectedDay, setSelectedDay] = useState<Date>(() => parseDateKey(initialDateKey) ?? new Date())
    // The URL date is the selected calendar day, NOT the real current time.
    // Until hydration, hide the clock indicator to avoid a false midnight line.
    const [currentTime, setCurrentTime] = useState<Date | null>(null)
    const [appointments, setAppointments] = useState<Appointment[]>(initialAppointments)
    const [calendarSettings, setCalendarSettings] = useState(initialSettings)
    const [selectedPatientNumber, setSelectedPatientNumber] = useState<number | null>(null)
    const {timeStepMinutes, displayStartHour, displayEndHour} = calendarSettings
    const [dialog, setDialog] = useState<DialogState | null>(null)
    const [clearDialogOpen, setClearDialogOpen] = useState(false)
    const [isClearingSchedule, setIsClearingSchedule] = useState(false)
    const [storageError, setStorageError] = useState<"read" | "write" | null>(null)
    const settingsRequestSequence = useRef(0)
    const appointmentsRequestSequence = useRef(0)
    const {trackRef, panelRef, viewportRef, contentRef, panelStyle} = usePageDrivenScroll({
        topOffset: 64,
        bottomGap: 16,
    })

    useEffect(() => {
        const frame = window.requestAnimationFrame(() => setCurrentTime(new Date()))
        const intervalId = window.setInterval(() => setCurrentTime(new Date()), 15_000)
        return () => {
            window.cancelAnimationFrame(frame)
            window.clearInterval(intervalId)
        }
    }, [])

    useEffect(() => {
        let active = true
        const refreshSettings = async () => {
            const requestSequence = ++settingsRequestSequence.current
            try {
                const settings = await getClientData<CalendarSettings>("/v1/calendar-settings")
                if (active && requestSequence === settingsRequestSequence.current) setCalendarSettings(settings)
            } catch {
                return
            }
        }

        void refreshSettings()
        window.addEventListener(CALENDAR_SETTINGS_CHANGE_EVENT, refreshSettings)
        return () => {
            active = false
            window.removeEventListener(CALENDAR_SETTINGS_CHANGE_EVENT, refreshSettings)
        }
    }, [])

    useEffect(() => {
        if (!selectedDate) return
        const requestSequence = ++appointmentsRequestSequence.current
        const start = addDays(selectedDate, -((selectedDate.getDay() + 6) % 7))
        const end = addDays(start, 7)
        const from = formatDateKey(start)
        const to = formatDateKey(end)
        getClientData<Appointment[]>(`/v1/appointments?from=${from}&to=${to}`)
            .then((loadedAppointments) => {
                if (requestSequence !== appointmentsRequestSequence.current) return
                setAppointments(loadedAppointments)
                // The patient filter is independent of the visible week.
                // In a week without their appointments, show an empty schedule
                // instead of silently switching to all patients.
                setStorageError(null)
            })
            .catch(() => {
                if (requestSequence === appointmentsRequestSequence.current) setStorageError("read")
            })
    }, [selectedDate])

    const handleClearSchedule = async () => {
        if (isClearingSchedule) return
        setIsClearingSchedule(true)
        ++appointmentsRequestSequence.current
        try {
            await sendBackendRequest("/v1/schedules", "DELETE", {confirmation: "CLEAR_ALL_SCHEDULES"})
            setAppointments([])
            setSelectedPatientNumber(null)
            setDialog(null)
            setStorageError(null)
            setClearDialogOpen(false)
            notify.success("Расписание очищено", "Все записи и планы пациентов удалены. Каталог процедур сохранён.")
            router.refresh()
        } catch {
            // Shared API error interceptor already notifies the user.
        } finally {
            setIsClearingSchedule(false)
        }
    }

    const handleSave = async (appointmentInput: AppointmentInput, id?: string) => {
        const appointmentId = id ?? crypto.randomUUID()
        try {
            const saved = await sendBackendRequest<Appointment>(`/v1/appointments/${appointmentId}`, "PUT", {
                ...appointmentInput,
                id: appointmentId,
                generatedByPlanner: id ? appointments.find((item) => item.id === id)?.generatedByPlanner ?? false : false,
                patientNumber: id ? appointments.find((item) => item.id === id)?.patientNumber : undefined,
            })
            if (!saved) throw new Error("Empty appointment response")
            setAppointments((current) => id ? current.map((item) => item.id === id ? saved : item) : [...current, saved])
            setStorageError(null)
            notify.success(id ? "Запись изменена" : "Запись добавлена", "Изменения сохранены в календаре.")
            setDialog(null)
        } catch {
            setStorageError("write")
        }
    }

    const handleDelete = async (id: string) => {
        try {
            await sendBackendRequest(`/v1/appointments/${id}`, "DELETE")
            setAppointments((current) => current.map((item) => item.id === id ? {...item, status: "cancelled"} : item))
            setStorageError(null)
            notify.success("Запись отменена", "Она больше не учитывается в расписании.")
            setDialog(null)
        } catch {
            setStorageError("write")
        }
    }

    const handleMoveAppointment = async (id: string, start: Date, end: Date) => {
        const existing = appointments.find((item) => item.id === id)
        if (!existing) return
        const isValidTime = canScheduleAt(calendarSettings.workWeek, start, end)
        const hasConflict = appointments.some((appointment) =>
            appointment.id !== id && appointment.status !== "cancelled" &&
            (existing.patientNumber == null || appointment.patientNumber == null || (existing.patientNumber === appointment.patientNumber && (!existing.sourcePlanDate || !appointment.sourcePlanDate || existing.sourcePlanDate === appointment.sourcePlanDate)) || (existing.room?.trim() && appointment.room?.trim() && existing.room.trim().toLocaleLowerCase() === appointment.room.trim().toLocaleLowerCase())) &&
            start < new Date(appointment.endAt) && end > new Date(appointment.startAt),
        )
        if (!isValidTime || hasConflict) {
            notify.warning(
                hasConflict ? "Это время уже занято" : "Время вне рабочего дня",
                hasConflict ? "Выберите свободное время или другой день." : "Выберите рабочий день и время, не пересекающееся с перерывами организации.",
            )
            return
        }
        try {
            const saved = await sendBackendRequest<Appointment>(`/v1/appointments/${id}`, "PUT", {
                ...existing,
                startAt: formatDateTimeLocal(start),
                endAt: formatDateTimeLocal(end)
            })
            if (saved) setAppointments((current) => current.map((item) => item.id === id ? saved : item))
            setStorageError(null)
            notify.success("Запись перенесена", "Новое время сохранено.")
        } catch {
            setStorageError("write")
        }
    }

    const handleRetryStorage = () => window.location.reload()
    const patientNumbers = [...new Set([
        ...appointments.flatMap((appointment) => appointment.patientNumber ? [appointment.patientNumber] : []),
        ...(selectedPatientNumber !== null ? [selectedPatientNumber] : []),
    ])].sort((first, second) => first - second)
    const visibleAppointments = selectedPatientNumber === null
        ? appointments
        : appointments.filter((appointment) => appointment.patientNumber === selectedPatientNumber)

    const handleCreate = () => {
        const now = currentTime ?? new Date()
        const date = selectedDay.toDateString() === now.toDateString() ? now : selectedDay
        const available = getDefaultStart(date, now, calendarSettings.workWeek)
        if (!available) {
            notify.warning("Нет доступного времени", "Проверьте часы работы и перерывы организации в настройках календаря.")
            return
        }
        setDialog({initialStartAt: formatDateTimeLocal(available)})
    }

    return (
        <>
            <div className="container mx-auto mb-4 w-full" ref={trackRef}>
                <main
                    className="sticky flex min-h-0 w-full flex-col overflow-clip rounded-[24px] bg-white text-slate-900"
                    ref={panelRef} style={panelStyle}>
                    <CalendarToolbar
                        date={selectedDate}
                        onSelectPatient={setSelectedPatientNumber}
                        patientNumbers={patientNumbers}
                        selectedPatientNumber={selectedPatientNumber}
                        onCreate={handleCreate}
                        onClearSchedule={() => setClearDialogOpen(true)}
                        isClearingSchedule={isClearingSchedule}
                        onMoveWeek={(offset) => {
                            setSelectedDate((date) => date && addDays(date, offset * 7))
                            setSelectedDay((date) => date && addDays(date, offset * 7))
                        }}
                        onToday={() => {
                            const today = new Date();
                            setSelectedDate(today);
                            setSelectedDay(today)
                        }}
                    />
                    <div className="min-h-0 flex-1 overflow-hidden">
                        <div className="h-full w-full">
                            {storageError && (
                                <p className="flex flex-wrap items-center justify-between gap-2 border-b border-amber-200 bg-amber-50 px-5 py-2 text-sm text-amber-900"
                                   role="alert">
                <span>
                  {storageError === "read"
                      ? "Не удалось загрузить записи. Проверьте соединение и попробуйте снова."
                      : "Последнее изменение не сохранено. Исправьте данные или попробуйте ещё раз."}
                </span>
                                    <Button
                                        className="h-auto shrink-0 rounded-lg px-2 py-1 font-medium underline underline-offset-2"
                                        onClick={handleRetryStorage}
                                        type="button"
                                        variant="link"
                                    >
                                        Повторить
                                    </Button>
                                </p>
                            )}
                            <div className="flex h-full flex-col">
                                {!storageError && visibleAppointments.length === 0 && (
                                    <p className="border-b border-slate-200 bg-slate-50 px-5 py-2 text-sm text-slate-600" role="status">
                                        {selectedPatientNumber === null
                                            ? "На выбранной неделе нет сохранённых записей. Проверьте дату или назначьте процедуры пациенту."
                                            : `У пациента ${selectedPatientNumber} нет записей на выбранной неделе.`}
                                    </p>
                                )}
                                <div className="min-h-0 flex-1">
                                    <WeekGrid
                                        appointments={visibleAppointments}
                                        currentTime={currentTime}
                                        date={selectedDate}
                                        selectedDay={selectedDay}
                                        onSelectAppointment={(appointment) =>
                                            setDialog({initialStartAt: appointment.startAt, appointment})
                                        }
                                        onMoveAppointment={handleMoveAppointment}
                                        onInvalidSelection={(message) => notify.warning("Не удалось выбрать время", message)}
                                        timeStepMinutes={timeStepMinutes}
                                        displayStartHour={displayStartHour}
                                        displayEndHour={displayEndHour}
                                        workWeek={calendarSettings.workWeek}
                                        viewportRef={viewportRef}
                                        contentRef={contentRef}
                                        onSelectTime={(date, endAt, anchor) => {
                                            setSelectedDay(date);
                                            setDialog({
                                                initialStartAt: formatDateTimeLocal(date),
                                                initialEndAt: endAt ? formatDateTimeLocal(endAt) : undefined,
                                                anchor
                                            })
                                        }}
                                        onSelectDay={setSelectedDay}
                                    />
                                </div>
                            </div>
                        </div>
                    </div>
                    <AppointmentDialog
                        appointment={dialog?.appointment}
                        appointments={appointments}
                        timeStepMinutes={timeStepMinutes}
                        workWeek={calendarSettings.workWeek}
                        initialStartAt={dialog?.initialStartAt ?? formatDateTimeLocal(currentTime ?? selectedDay)}
                        initialEndAt={dialog?.initialEndAt}
                        anchor={dialog?.anchor}
                        onClose={() => setDialog(null)}
                        onDelete={handleDelete}
                        onSave={handleSave}
                        open={dialog !== null}
                    />
                    <ClearScheduleDialog
                        open={clearDialogOpen}
                        pending={isClearingSchedule}
                        onClose={() => setClearDialogOpen(false)}
                        onConfirm={() => void handleClearSchedule()}
                    />
                </main>
            </div>
        </>
    )
}
