import type {Appointment, AppointmentColor} from "@/entities/appointment"
import type {TimeStepMinutes} from "@/entities/calendar-settings"
import {useEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type RefObject} from "react"
import {createPortal} from "react-dom"
import {
    LUNCH_END_HOUR,
    LUNCH_START_HOUR,
    formatDateKey,
    getWeekDays,
    HOUR_HEIGHT,
    WORKDAY_END_HOUR,
    WORKDAY_START_HOUR,
} from "@/shared/lib/calendar-date"
import {Button} from "@/shared/ui/shadcn/button"

type WeekGridProps = {
    date: Date
    selectedDay: Date
    appointments: Appointment[]
    currentTime: Date
    onSelectTime: (date: Date, endAt: Date | undefined, anchor: DOMRect) => void
    onSelectDay: (date: Date) => void
    onSelectAppointment: (appointment: Appointment) => void
    onMoveAppointment: (id: string, start: Date, end: Date) => void
    onInvalidSelection: (message: string) => void
    timeStepMinutes: TimeStepMinutes
    displayStartHour: number
    displayEndHour: number
    availableHeight: number
    viewportRef: RefObject<HTMLDivElement | null>
    contentRef: RefObject<HTMLDivElement | null>
}

const weekdayFormatter = new Intl.DateTimeFormat("ru-RU", {weekday: "short"})

const eventColors: Record<AppointmentColor, string> = {
    blue: "bg-blue-100 text-blue-950 hover:bg-blue-200",
    teal: "bg-teal-100 text-teal-950 hover:bg-teal-200",
    orange: "bg-orange-100 text-orange-950 hover:bg-orange-200",
    purple: "bg-violet-100 text-violet-950 hover:bg-violet-200",
}

type DragPreview = {appointmentId: string; dayKey: string; start: Date; end: Date; left: number; top: number; width: number; height: number}
type AppointmentMove = {appointmentId: string; pointerId: number; initialX: number; initialY: number; grabOffsetMinutes: number; mode: "move" | "resize-start" | "resize-end"; hasMoved: boolean}

function timeLabel(date: Date) {
    return `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`
}

function layoutOverlappingAppointments(appointments: Appointment[]) {
    const sortedAppointments = [...appointments].sort(
        (first, second) => new Date(first.startAt).getTime() - new Date(second.startAt).getTime(),
    )
    const layouts = new Map<string, { column: number; columns: number }>()
    let group: Appointment[] = []
    let groupEnd = 0

    const placeGroup = () => {
        const columnEnds: number[] = []
        const placements: { id: string; column: number }[] = []

        group.forEach((appointment) => {
            const start = new Date(appointment.startAt).getTime()
            const end = new Date(appointment.endAt).getTime()
            let column = columnEnds.findIndex((columnEnd) => columnEnd <= start)
            if (column === -1) column = columnEnds.length
            columnEnds[column] = end
            placements.push({id: appointment.id, column})
        })

        placements.forEach(({id, column}) =>
            layouts.set(id, {column, columns: columnEnds.length}),
        )
        group = []
    }

    sortedAppointments.forEach((appointment) => {
        const start = new Date(appointment.startAt).getTime()
        const end = new Date(appointment.endAt).getTime()
        if (group.length > 0 && start >= groupEnd) placeGroup()
        group.push(appointment)
        groupEnd = Math.max(groupEnd, end)
    })
    if (group.length > 0) placeGroup()

    return layouts
}

export function WeekGrid({
                             date,
                             selectedDay,
                             appointments,
                             currentTime,
                             onSelectTime,
                             onSelectDay,
                             onSelectAppointment,
                             onMoveAppointment,
                             onInvalidSelection,
                             timeStepMinutes,
                             displayStartHour,
                             displayEndHour,
                             availableHeight,
                             viewportRef,
                             contentRef,
                         }: WeekGridProps) {
    const hours = Array.from({length: displayEndHour - displayStartHour}, (_, index) => displayStartHour + index)
    const [selection, setSelection] = useState<{dayKey: string; startMinutes: number; endMinutes: number} | null>(null)
    const [draggedAppointmentId, setDraggedAppointmentId] = useState<string | null>(null)
    const [dragPreview, setDragPreview] = useState<DragPreview | null>(null)
    const activeMove = useRef<AppointmentMove | null>(null)
    const dragPreviewRef = useRef<DragPreview | null>(null)
    const suppressAppointmentClick = useRef<string | null>(null)
    const selectionOrigin = useRef<{dayKey: string; minutes: number; y: number; pointerId: number} | null>(null)
    const ignoreNextSlotClick = useRef(false)
    const hourHeight = availableHeight ? Math.max(48, availableHeight / hours.length) : HOUR_HEIGHT

    useEffect(() => {
        const clearTransientInteractions = () => {
            if (activeMove.current?.hasMoved) {
                suppressAppointmentClick.current = activeMove.current.appointmentId
                window.setTimeout(() => { suppressAppointmentClick.current = null }, 0)
            }
            activeMove.current = null
            dragPreviewRef.current = null
            selectionOrigin.current = null
            setDraggedAppointmentId(null)
            setDragPreview(null)
            setSelection(null)
        }
        const handleVisibilityChange = () => {
            if (document.visibilityState === "hidden") clearTransientInteractions()
        }

        window.addEventListener("blur", clearTransientInteractions)
        document.addEventListener("visibilitychange", handleVisibilityChange)
        return () => {
            window.removeEventListener("blur", clearTransientInteractions)
            document.removeEventListener("visibilitychange", handleVisibilityChange)
        }
    }, [])

    const days = getWeekDays(date)
    const timezone =
        new Intl.DateTimeFormat("ru-RU", {timeZoneName: "short"})
            .formatToParts(currentTime)
            .find((part) => part.type === "timeZoneName")?.value ?? ""
    const nowMinutes = currentTime.getHours() * 60 + currentTime.getMinutes()
    const isCurrentTimeVisible =
        nowMinutes >= displayStartHour * 60 && nowMinutes < displayEndHour * 60
    const nowTop = ((nowMinutes - displayStartHour * 60) / 60) * hourHeight

    const beginAppointmentGesture = (
        event: ReactPointerEvent<HTMLElement>,
        appointment: Appointment,
        mode: AppointmentMove["mode"],
        hiddenDuration = 0,
    ) => {
        if (appointment.status === "cancelled" || event.button !== 0) return
        event.preventDefault()
        event.stopPropagation()
        const bounds = event.currentTarget.getBoundingClientRect()
        const appointmentBounds = event.currentTarget.closest("[data-appointment-card]")?.getBoundingClientRect() ?? bounds
        const pointerOffset = (event.clientY - appointmentBounds.top) / hourHeight * 60
        const grabOffsetMinutes = mode === "resize-end"
            ? (event.clientY - appointmentBounds.bottom) / hourHeight * 60
            : pointerOffset + hiddenDuration
        activeMove.current = {
            appointmentId: appointment.id,
            pointerId: event.pointerId,
            initialX: event.clientX,
            initialY: event.clientY,
            grabOffsetMinutes,
            mode,
            hasMoved: false,
        }
        event.currentTarget.setPointerCapture(event.pointerId)
    }

    const handleMovePointer = (event: ReactPointerEvent<HTMLDivElement>) => {
        const move = activeMove.current
        if (!move || move.pointerId !== event.pointerId) return
        if (!move.hasMoved && Math.hypot(event.clientX - move.initialX, event.clientY - move.initialY) < 6) return
        move.hasMoved = true
        const dayElement = document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>("[data-calendar-day]")
        const dayKey = dayElement?.dataset.calendarDay
        const targetDay = days.find((item) => formatDateKey(item) === dayKey)
        const appointment = appointments.find((item) => item.id === move.appointmentId)
        if (!dayElement || !targetDay || !appointment) {
            setDraggedAppointmentId(move.appointmentId)
            dragPreviewRef.current = null
            setDragPreview(null)
            return
        }
        const bounds = dayElement.getBoundingClientRect()
        const originalStart = new Date(appointment.startAt)
        const originalEnd = new Date(appointment.endAt)
        const originalStartMinutes = originalStart.getHours() * 60 + originalStart.getMinutes()
        const originalEndMinutes = originalEnd.getHours() * 60 + originalEnd.getMinutes()
        const originalDuration = originalEnd.getTime() - originalStart.getTime()
        const targetDayKey = formatDateKey(targetDay)
        const originalDayKey = formatDateKey(originalStart)
        if (move.mode !== "move" && targetDayKey !== originalDayKey) {
            dragPreviewRef.current = null
            setDragPreview(null)
            return
        }

        const pointerMinutes = displayStartHour * 60 + ((event.clientY - bounds.top) / hourHeight) * 60
        const rawEdgeMinutes = pointerMinutes - move.grabOffsetMinutes
        let start: Date
        let end: Date

        if (move.mode === "resize-start") {
            const previousAppointments = appointments
                .filter((item) => item.id !== appointment.id && item.status !== "cancelled" && (appointment.patientNumber == null || item.patientNumber == null || (appointment.patientNumber === item.patientNumber && (!appointment.sourcePlanDate || !item.sourcePlanDate || appointment.sourcePlanDate === item.sourcePlanDate))) && formatDateKey(new Date(item.startAt)) === originalDayKey)
                .map((item) => new Date(item.endAt))
                .filter((itemEnd) => itemEnd.getHours() * 60 + itemEnd.getMinutes() <= originalStartMinutes)
            const lunchStartBound = originalEndMinutes > LUNCH_START_HOUR * 60 ? LUNCH_END_HOUR * 60 : WORKDAY_START_HOUR * 60
            const lowerBound = Math.max(WORKDAY_START_HOUR * 60, lunchStartBound, ...previousAppointments.map((itemEnd) => itemEnd.getHours() * 60 + itemEnd.getMinutes()))
            const minimumStart = Math.ceil(lowerBound / timeStepMinutes) * timeStepMinutes
            const maximumStart = Math.floor((originalEndMinutes - timeStepMinutes) / timeStepMinutes) * timeStepMinutes
            if (minimumStart > maximumStart) {
                dragPreviewRef.current = null
                setDragPreview(null)
                return
            }
            const snappedStart = Math.round(rawEdgeMinutes / timeStepMinutes) * timeStepMinutes
            const startMinutes = Math.max(minimumStart, Math.min(maximumStart, snappedStart))
            start = new Date(originalStart)
            start.setHours(0, startMinutes, 0, 0)
            end = originalEnd
        } else if (move.mode === "resize-end") {
            const nextAppointments = appointments
                .filter((item) => item.id !== appointment.id && item.status !== "cancelled" && (appointment.patientNumber == null || item.patientNumber == null || (appointment.patientNumber === item.patientNumber && (!appointment.sourcePlanDate || !item.sourcePlanDate || appointment.sourcePlanDate === item.sourcePlanDate))) && formatDateKey(new Date(item.startAt)) === originalDayKey)
                .map((item) => new Date(item.startAt))
                .filter((itemStart) => itemStart.getHours() * 60 + itemStart.getMinutes() >= originalEndMinutes)
            const lunchEndBound = originalStartMinutes < LUNCH_END_HOUR * 60 ? LUNCH_START_HOUR * 60 : WORKDAY_END_HOUR * 60
            const upperBound = Math.min(WORKDAY_END_HOUR * 60, lunchEndBound, ...nextAppointments.map((itemStart) => itemStart.getHours() * 60 + itemStart.getMinutes()))
            const minimumEnd = Math.ceil((originalStartMinutes + timeStepMinutes) / timeStepMinutes) * timeStepMinutes
            const maximumEnd = Math.floor(upperBound / timeStepMinutes) * timeStepMinutes
            if (minimumEnd > maximumEnd) {
                dragPreviewRef.current = null
                setDragPreview(null)
                return
            }
            const snappedEnd = Math.round(rawEdgeMinutes / timeStepMinutes) * timeStepMinutes
            const endMinutes = Math.max(minimumEnd, Math.min(maximumEnd, snappedEnd))
            start = originalStart
            end = new Date(originalEnd)
            end.setHours(0, endMinutes, 0, 0)
        } else {
            const durationSteps = Math.ceil(originalDuration / (timeStepMinutes * 60_000)) * timeStepMinutes
            const latestStart = Math.max(WORKDAY_START_HOUR * 60, WORKDAY_END_HOUR * 60 - durationSteps)
            const snappedMinutes = Math.round(rawEdgeMinutes / timeStepMinutes) * timeStepMinutes
            const startMinutes = Math.max(WORKDAY_START_HOUR * 60, Math.min(latestStart, snappedMinutes))
            start = new Date(targetDay)
            start.setHours(0, startMinutes, 0, 0)
            end = new Date(start.getTime() + originalDuration)
        }
        const startMinutes = start.getHours() * 60 + start.getMinutes()
        const preview: DragPreview = {
            appointmentId: appointment.id,
            dayKey: targetDayKey,
            start,
            end,
            left: bounds.left + 4,
            top: bounds.top + ((startMinutes - displayStartHour * 60) / 60) * hourHeight,
            width: bounds.width - 8,
            height: Math.max(26, ((end.getTime() - start.getTime()) / 60_000) * hourHeight / 60 - 3),
        }
        setDraggedAppointmentId(appointment.id)
        dragPreviewRef.current = preview
        setDragPreview(preview)
    }

    const finishMove = (event: ReactPointerEvent<HTMLDivElement>, cancel = false) => {
        const move = activeMove.current
        if (!move || move.pointerId !== event.pointerId) return
        const preview = dragPreviewRef.current
        if (!cancel && move.hasMoved) {
            suppressAppointmentClick.current = move.appointmentId
            window.setTimeout(() => { suppressAppointmentClick.current = null }, 0)
            if (preview?.appointmentId === move.appointmentId) {
                onMoveAppointment(move.appointmentId, preview.start, preview.end)
            }
        }
        activeMove.current = null
        dragPreviewRef.current = null
        setDraggedAppointmentId(null)
        setDragPreview(null)
    }

    const previewAppointment = dragPreview
        ? appointments.find((appointment) => appointment.id === dragPreview.appointmentId)
        : undefined

    return (
        <>
        <div
            className="h-full overflow-x-auto overflow-y-hidden bg-white"
            onLostPointerCapture={(event) => finishMove(event, true)}
            onPointerCancel={(event) => finishMove(event, true)}
            onPointerMove={handleMovePointer}
            onPointerUp={(event) => {
                if (activeMove.current?.pointerId === event.pointerId) handleMovePointer(event)
                finishMove(event)
            }}
        >
            <div className="flex h-full min-w-245 flex-col">
                <div
                    className="sticky top-0 z-20 grid shrink-0 grid-cols-[58px_repeat(7,minmax(0,1fr))] gap-x-px border-b border-slate-200 bg-slate-200">
                    <div
                        className="flex items-end justify-center bg-white pb-3 text-[10px] font-medium uppercase tracking-wide text-slate-400">
                        {timezone}
                    </div>
                    {days.map((day) => {
                        const isToday = formatDateKey(day) === formatDateKey(currentTime)
                        const isSelected = formatDateKey(day) === formatDateKey(selectedDay)
                        return (
                            <div className="flex h-[76px] flex-col items-center justify-center gap-1 bg-white"
                                 key={formatDateKey(day)}>
                <span
                    className={`text-[11px] font-medium uppercase tracking-wider ${isToday ? "text-blue-600" : "text-slate-500"}`}>
                  {weekdayFormatter.format(day).replace(".", "")}
                </span>
                                <Button aria-label={`Выбрать ${formatDateKey(day)}`}
                                        className={`size-10 rounded-full p-0 text-xl font-medium tabular-nums ${isToday ? "bg-blue-600 text-white shadow-md shadow-blue-200 hover:bg-blue-700" : isSelected ? "bg-blue-100 text-blue-800 ring-2 ring-blue-300 hover:bg-blue-200" : "text-slate-800 hover:bg-slate-100"}`}
                                        onClick={() => onSelectDay(day)} type="button" variant="ghost">
                                    {day.getDate()}
                                </Button>
                            </div>
                        )
                    })}
                </div>

                <div className="min-h-0 flex-1 overflow-hidden" ref={viewportRef}>
                <div ref={contentRef} className="grid grid-cols-[58px_repeat(7,minmax(0,1fr))] gap-x-px bg-slate-200" style={{ minHeight: hours.length * hourHeight }}>
                    <div className="relative grid bg-white" style={{ gridTemplateRows: `repeat(${hours.length}, ${hourHeight - 1}px)`, rowGap: 1, paddingBottom: 1 }}>
                        {hours.map((hour) => (
                            <div
                                className="relative text-right"
                                key={hour}
                                style={{minHeight: hourHeight - 1}}
                            >
                <span className={`absolute right-2 text-[11px] tabular-nums text-slate-400 ${hour === displayStartHour ? "top-1" : "-top-2.5"}`}>
                  {String(hour).padStart(2, "0")}:00
                </span>
                            </div>
                        ))}
                        <span className="absolute right-2 bottom-1 text-[11px] tabular-nums text-slate-400">{displayEndHour === 24 ? "23:59" : `${String(displayEndHour).padStart(2, "0")}:00`}</span>
                    </div>

                    {days.map((day) => {
                        const dayKey = formatDateKey(day)
                        const dayStart = new Date(day)
                        dayStart.setHours(displayStartHour, 0, 0, 0)
                        const dayEnd = new Date(day)
                        dayEnd.setHours(displayEndHour, 0, 0, 0)
                        const dayAppointments = appointments.filter((appointment) => {
                            const start = new Date(appointment.startAt)
                            const end = new Date(appointment.endAt)
                            return start < dayEnd && end > dayStart
                        })
                        const appointmentLayouts = layoutOverlappingAppointments(dayAppointments)
                        const isToday = dayKey === formatDateKey(currentTime)

                        return (
                            <div
                                className="relative grid grid-flow-row bg-slate-200"
                                data-calendar-day={dayKey}
                                key={dayKey}
                                style={{gridTemplateRows: `repeat(${hours.length}, ${hourHeight - 1}px)`, rowGap: 1, paddingBottom: 1}}
                                onPointerDown={(event) => {
                                    if (!(event.target instanceof HTMLElement) || !event.target.closest("[data-calendar-slot]")) return
                                    const slot = event.target.closest<HTMLElement>("[data-calendar-slot]")
                                    const slotMinutes = Number(slot?.dataset.calendarSlot)
                                    if (!Number.isFinite(slotMinutes) || !slot) return
                                    const slotBounds = slot.getBoundingClientRect()
                                    const minutes = slotMinutes + Math.round(((event.clientY - slotBounds.top) / hourHeight) * 60 / timeStepMinutes) * timeStepMinutes
                                    selectionOrigin.current = {dayKey, minutes, y: event.clientY, pointerId: event.pointerId}
                                    slot?.setPointerCapture(event.pointerId)
                                }}
                                onPointerMove={(event) => {
                                    const origin = selectionOrigin.current
                                    if (!origin || origin.dayKey !== dayKey || origin.pointerId !== event.pointerId) return
                                    if (Math.abs(event.clientY - origin.y) < 6) return
                                    const bounds = event.currentTarget.getBoundingClientRect()
                                    const minutes = Math.max(displayStartHour * 60, Math.min(displayEndHour * 60, displayStartHour * 60 + Math.round(((event.clientY - bounds.top) / hourHeight) * 60 / timeStepMinutes) * timeStepMinutes))
                                    const startMinutes = Math.min(origin.minutes, minutes)
                                    setSelection({dayKey, startMinutes, endMinutes: Math.max(startMinutes + 30, origin.minutes, minutes)})
                                }}
                                onPointerUp={(event) => {
                                    const origin = selectionOrigin.current
                                    if (!origin || origin.dayKey !== dayKey || origin.pointerId !== event.pointerId) return
                                    const bounds = event.currentTarget.getBoundingClientRect()
                                    const minutes = Math.max(displayStartHour * 60, Math.min(displayEndHour * 60, displayStartHour * 60 + Math.round(((event.clientY - bounds.top) / hourHeight) * 60 / timeStepMinutes) * timeStepMinutes))
                                    const startMinutes = Math.min(origin.minutes, minutes)
                                    const endMinutes = Math.max(startMinutes + 30, origin.minutes, minutes)
                                    const dragged = Math.abs(event.clientY - origin.y) >= 6
                                    selectionOrigin.current = null
                                    setSelection(null)
                                    if (!dragged) return
                                    ignoreNextSlotClick.current = true
                                    window.setTimeout(() => { ignoreNextSlotClick.current = false }, 0)
                                    const start = new Date(day)
                                    start.setHours(0, startMinutes, 0, 0)
                                    const end = new Date(day)
                                    end.setHours(0, endMinutes, 0, 0)
                                    if (startMinutes < WORKDAY_START_HOUR * 60 || endMinutes > WORKDAY_END_HOUR * 60 || (startMinutes < LUNCH_END_HOUR * 60 && endMinutes > LUNCH_START_HOUR * 60)) {
                                        onInvalidSelection("Выберите интервал в рабочее время 09:00–16:00, кроме обеда 13:00–14:00.")
                                        return
                                    }
                                    onSelectTime(
                                        start,
                                        end,
                                        new DOMRect(
                                            bounds.left,
                                            bounds.top + ((startMinutes - displayStartHour * 60) / 60) * hourHeight,
                                            bounds.width,
                                            Math.max(1, ((endMinutes - startMinutes) / 60) * hourHeight),
                                        ),
                                    )
                                }}
                                onPointerCancel={(event) => {
                                    if (selectionOrigin.current?.pointerId !== event.pointerId) return
                                    selectionOrigin.current = null
                                    setSelection(null)
                                }}
                                onLostPointerCapture={(event) => {
                                    if (selectionOrigin.current?.pointerId !== event.pointerId) return
                                    selectionOrigin.current = null
                                    setSelection(null)
                                }}
                            >
                                {hours.map((hour) => {
                                    const slot = new Date(day)
                                    slot.setHours(hour, 0, 0, 0)
                                    const slotMinutes = hour * 60
                                    const isLunchSlot = slotMinutes >= LUNCH_START_HOUR * 60 && slotMinutes < LUNCH_END_HOUR * 60
                                    const unavailable = slotMinutes < WORKDAY_START_HOUR * 60 || (slotMinutes < LUNCH_END_HOUR * 60 && slotMinutes + 50 > LUNCH_START_HOUR * 60) || slotMinutes + 50 > WORKDAY_END_HOUR * 60
                                    return (
                                        <Button
                                            data-calendar-slot={slotMinutes}
                                            aria-label={`Создать запись: ${dayKey}, ${timeLabel(slot)}`}
                                            className={`block w-full rounded-none border-0 text-left transition-colors ${isLunchSlot ? "disabled:opacity-100" : ""} ${unavailable ? `cursor-not-allowed ${isLunchSlot ? "bg-white" : "bg-slate-50"}` : "bg-white hover:bg-blue-50/70"}`}
                                            disabled={unavailable}
                                            key={hour}
                                            onClick={(event) => {
                                                if (ignoreNextSlotClick.current) {
                                                    ignoreNextSlotClick.current = false
                                                    return
                                                }
                                                onSelectTime(slot, undefined, event.currentTarget.getBoundingClientRect())
                                            }}
                                            style={{height: "100%"}}
                                            type="button"
                                            variant="ghost"
                                        />
                                    )
                                })}

                                {selection?.dayKey === dayKey && (
                                    <div
                                        className="pointer-events-none absolute z-30 overflow-hidden rounded-lg bg-blue-100/95 px-2 py-1 text-xs text-blue-950"
                                        style={{top: (selection.startMinutes - displayStartHour * 60) * hourHeight / 60, height: Math.max(28, (selection.endMinutes - selection.startMinutes) * hourHeight / 60), left: 5, right: 5}}
                                    >
                                        <strong className="block truncate">Новая запись</strong>
                                        <span>{String(Math.floor(selection.startMinutes / 60)).padStart(2, "0")}:{String(selection.startMinutes % 60).padStart(2, "0")} – {String(Math.floor(selection.endMinutes / 60)).padStart(2, "0")}:{String(selection.endMinutes % 60).padStart(2, "0")}</span>
                                    </div>
                                )}

                                <div
                                    aria-label="Обеденный перерыв"
                                    className="pointer-events-none absolute z-[5] flex flex-col items-start justify-start overflow-hidden rounded-lg border-0 bg-teal-100 px-2 py-1 text-left text-xs text-teal-950 shadow-none ring-0 outline-none"
                                    style={{
                                        top: (LUNCH_START_HOUR - displayStartHour) * hourHeight + 4,
                                        height: (LUNCH_END_HOUR - LUNCH_START_HOUR) * hourHeight - 8,
                                        left: 4,
                                        right: 4,
                                    }}
                                >
                                    <strong className="truncate font-semibold">Обед</strong>
                                    <span className="tabular-nums">13:00 – 14:00</span>
                                </div>

                                {dayAppointments.map((appointment) => {
                                    const start = new Date(appointment.startAt)
                                    const end = new Date(appointment.endAt)
                                    const visibleStart = start < dayStart ? dayStart : start
                                    const visibleEnd = end > dayEnd ? dayEnd : end
                                    if (visibleEnd <= visibleStart) return null
                                    const startMinutes = visibleStart.getHours() * 60 + visibleStart.getMinutes()
                                    const endMinutes =
                                        visibleEnd.getTime() === dayEnd.getTime()
                                            ? displayEndHour * 60
                                            : visibleEnd.getHours() * 60 + visibleEnd.getMinutes()
                                    const top = Math.max(
                                        0,
                                        ((startMinutes - displayStartHour * 60) / 60) * hourHeight,
                                    )
                                    const bottom = Math.min(
                                        (displayEndHour - displayStartHour) * hourHeight,
                                        ((endMinutes - displayStartHour * 60) / 60) * hourHeight,
                                    )
                                    const height = Math.max(26, bottom - top - 3)
                                    const layout = appointmentLayouts.get(appointment.id) ?? {
                                        column: 0,
                                        columns: 1,
                                    }
                                    const width = 100 / layout.columns

                                    const isMoving = draggedAppointmentId === appointment.id && dragPreview?.appointmentId === appointment.id
                                    const displayedStart = isMoving ? dragPreview.start : visibleStart
                                    const displayedEnd = isMoving ? dragPreview.end : visibleEnd
                                    return (
                                        <Button
                                            aria-hidden={isMoving || undefined}
                                            className={`calendar-appointment-card absolute z-10 group flex-col items-start justify-start touch-none overflow-hidden rounded-lg px-2 py-1 text-left text-xs transition ${eventColors[appointment.color]} ${appointment.status === "cancelled" ? "opacity-60 grayscale" : "cursor-grab active:cursor-grabbing"} ${isMoving ? "invisible" : ""}`}
                                            key={appointment.id}
                                            data-appointment-card
                                            onPointerDown={(event) => beginAppointmentGesture(event, appointment, "move", start < dayStart ? (dayStart.getTime() - start.getTime()) / 60_000 : 0)}
                                            onClick={(event) => {
                                                if (suppressAppointmentClick.current === appointment.id) {
                                                    event.preventDefault()
                                                    event.stopPropagation()
                                                    suppressAppointmentClick.current = null
                                                    return
                                                }
                                                event.stopPropagation()
                                                onSelectAppointment(appointment)
                                            }}
                                            style={{top, height, left: `calc(${layout.column * width}% + 4px)`, width: `calc(${width}% - 8px)`}}
                                            type="button"
                                            variant="ghost"
                                        >
                                            <span className="calendar-appointment-content">
                                                <span className="calendar-appointment-title font-semibold">{appointment.status === "cancelled" ? "Отменено · " : ""}{appointment.title}</span>
                                                <span className="calendar-appointment-time tabular-nums">
                                                    {timeLabel(displayedStart)} – {timeLabel(displayedEnd)}
                                                </span>
                                            </span>
                                            {appointment.status !== "cancelled" && (
                                                <>
                                                    <span
                                                        aria-hidden="true"
                                                        className="absolute inset-x-0 top-0 z-20 h-2 cursor-ns-resize opacity-0 transition-opacity group-hover:opacity-100 before:absolute before:left-1/2 before:top-1 before:h-0.5 before:w-6 before:-translate-x-1/2 before:rounded-full before:bg-current before:opacity-50 hover:opacity-100"
                                                        onPointerDown={(event) => beginAppointmentGesture(event, appointment, "resize-start")}
                                                    />
                                                    <span
                                                        aria-hidden="true"
                                                        className="absolute inset-x-0 bottom-0 z-20 h-2 cursor-ns-resize opacity-0 transition-opacity group-hover:opacity-100 before:absolute before:bottom-1 before:left-1/2 before:h-0.5 before:w-6 before:-translate-x-1/2 before:rounded-full before:bg-current before:opacity-50 hover:opacity-100"
                                                        onPointerDown={(event) => beginAppointmentGesture(event, appointment, "resize-end")}
                                                    />
                                                </>
                                            )}
                                        </Button>
                                    )
                                })}

                                {isToday && isCurrentTimeVisible && (
                                    <div
                                        className="pointer-events-none absolute left-0 right-0 z-10 border-t-2 border-red-500"
                                        style={{top: nowTop}}>
                                        <span className="absolute -left-1 -top-[5px] size-2.5 rounded-full bg-red-500"/>
                                    </div>
                                )}
                            </div>
                        )
                    })}
                </div>
                </div>
            </div>
        </div>
        {dragPreview && previewAppointment && typeof document !== "undefined" && createPortal(
            <div
                aria-hidden="true"
                className={`calendar-appointment-card pointer-events-none fixed z-50 flex flex-col items-start justify-start overflow-hidden rounded-lg px-2 py-1 text-left text-xs ${eventColors[previewAppointment.color]} ${previewAppointment.status === "cancelled" ? "opacity-60 grayscale" : ""}`}
                style={{
                    top: dragPreview.top,
                    left: dragPreview.left,
                    height: dragPreview.height,
                    width: dragPreview.width,
                }}
            >
                <span className="calendar-appointment-content">
                    <span className="calendar-appointment-title font-semibold">{previewAppointment.status === "cancelled" ? "Отменено · " : ""}{previewAppointment.title}</span>
                    <span className="calendar-appointment-time tabular-nums">
                        {timeLabel(dragPreview.start)} – {timeLabel(dragPreview.end)}
                    </span>
                </span>
            </div>,
            document.body,
        )}
        </>
    )
}
