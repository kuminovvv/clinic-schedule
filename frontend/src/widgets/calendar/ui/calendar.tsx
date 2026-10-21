"use client"

import {useEffect, useRef, useState} from "react"
import {
    type Appointment,
    type AppointmentInput,
} from "@/entities/appointment"
import {CALENDAR_SETTINGS_CHANGE_EVENT, type TimeStepMinutes} from "@/entities/calendar-settings"
import {AppointmentDialog} from "@/features/manage-appointment/ui/appointment-dialog"
import {
    addDays,
    formatDateKey,
    formatDateTimeLocal,
    LUNCH_END_HOUR,
    LUNCH_START_HOUR,
    WORKDAY_END_HOUR,
    WORKDAY_START_HOUR
} from "@/shared/lib/calendar-date"
import {getClientData, sendBackendRequest} from "@/shared/api/backend-client"
import {notify} from "@/shared/lib/notifications"
import {Button} from "@/shared/ui/shadcn/button"
import {CalendarToolbar} from "./calendar-toolbar"
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

function getDefaultStart(date: Date, currentTime: Date) {
    const start = new Date(date)
    if (start.toDateString() !== currentTime.toDateString()) {
        start.setHours(9, 0, 0, 0)
        return start
    }

    start.setTime(currentTime.getTime())
    start.setSeconds(0, 0)
    const nextSlot = (Math.floor((currentTime.getHours() * 60 + currentTime.getMinutes()) / 30) + 1) * 30
    start.setHours(0, 0, 0, 0)
    start.setMinutes(nextSlot)
    const minutes = start.getHours() * 60 + start.getMinutes()
    if (minutes >= WORKDAY_END_HOUR * 60) {
        start.setDate(start.getDate() + 1)
        start.setHours(9, 0, 0, 0)
    } else if (minutes < 9 * 60) start.setHours(9, 0, 0, 0)
    if (minutes >= LUNCH_START_HOUR * 60 && minutes < LUNCH_END_HOUR * 60) start.setHours(LUNCH_END_HOUR, 0, 0, 0)
    return start
}

type CalendarSettings = { timeStepMinutes: TimeStepMinutes; displayStartHour: number; displayEndHour: number }

export function Calendar({initialDateKey, initialAppointments, initialSettings}: {
    initialDateKey: string;
    initialAppointments: Appointment[];
    initialSettings: CalendarSettings
}) {
    const [selectedDate, setSelectedDate] = useState<Date>(() => parseDateKey(initialDateKey) ?? new Date())
    const [selectedDay, setSelectedDay] = useState<Date>(() => parseDateKey(initialDateKey) ?? new Date())
    const [currentTime, setCurrentTime] = useState<Date>(() => parseDateKey(initialDateKey) ?? new Date())
    const [appointments, setAppointments] = useState<Appointment[]>(initialAppointments)
    const [calendarSettings, setCalendarSettings] = useState(initialSettings)
    const [selectedPatientNumber, setSelectedPatientNumber] = useState<number | null>(null)
    const {timeStepMinutes, displayStartHour, displayEndHour} = calendarSettings
    const [dialog, setDialog] = useState<DialogState | null>(null)
    const [storageError, setStorageError] = useState<"read" | "write" | null>(null)
    const settingsRequestSequence = useRef(0)
    const {trackRef, panelRef, viewportRef, contentRef, viewportHeight, panelStyle} = usePageDrivenScroll({
        topOffset: 64,
        bottomGap: 16,
    })

    useEffect(() => {
        const intervalId = window.setInterval(() => setCurrentTime(new Date()), 60_000)
        return () => window.clearInterval(intervalId)
    }, [initialDateKey])

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
        const start = addDays(selectedDate, -((selectedDate.getDay() + 6) % 7))
        const end = addDays(start, 7)
        const from = formatDateKey(start)
        const to = formatDateKey(end)
        getClientData<Appointment[]>(`/v1/appointments?from=${from}&to=${to}`)
            .then((loadedAppointments) => {
                setAppointments(loadedAppointments)
                setStorageError(null)
            })
            .catch(() => setStorageError("read"))
    }, [selectedDate])

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
        const startMinutes = start.getHours() * 60 + start.getMinutes()
        const endMinutes = end.getHours() * 60 + end.getMinutes()
        const isValidTime = start < end && start.toDateString() === end.toDateString() &&
            startMinutes >= WORKDAY_START_HOUR * 60 &&
            endMinutes <= WORKDAY_END_HOUR * 60 &&
            !(startMinutes < LUNCH_END_HOUR * 60 && endMinutes > LUNCH_START_HOUR * 60)
        const hasConflict = appointments.some((appointment) =>
            appointment.id !== id && appointment.status !== "cancelled" &&
            (existing.patientNumber == null || appointment.patientNumber == null || (existing.patientNumber === appointment.patientNumber && (!existing.sourcePlanDate || !appointment.sourcePlanDate || existing.sourcePlanDate === appointment.sourcePlanDate))) &&
            start < new Date(appointment.endAt) && end > new Date(appointment.startAt),
        )
        if (!isValidTime || hasConflict) {
            notify.warning(
                hasConflict ? "Это время уже занято" : "Время вне рабочего дня",
                hasConflict ? "Выберите свободное время или другой день." : "Выберите время с 09:00 до 16:00, кроме перерыва с 13:00 до 14:00.",
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
    const patientNumbers = [...new Set(appointments.flatMap((appointment) => appointment.patientNumber ? [appointment.patientNumber] : []))].sort((first, second) => first - second)
    const visibleAppointments = selectedPatientNumber === null
        ? appointments
        : appointments.filter((appointment) => appointment.patientNumber === selectedPatientNumber)

    const handleCreate = () => {
        const date = selectedDay.toDateString() === currentTime.toDateString() ? currentTime : selectedDay
        setDialog({initialStartAt: formatDateTimeLocal(getDefaultStart(date, currentTime))})
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
                                        availableHeight={viewportHeight}
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
                        initialStartAt={dialog?.initialStartAt ?? formatDateTimeLocal(currentTime)}
                        initialEndAt={dialog?.initialEndAt}
                        anchor={dialog?.anchor}
                        onClose={() => setDialog(null)}
                        onDelete={handleDelete}
                        onSave={handleSave}
                        open={dialog !== null}
                    />
                </main>
            </div>
        </>
    )
}
