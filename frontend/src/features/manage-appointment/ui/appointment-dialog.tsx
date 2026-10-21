"use client"

import { useEffect, useState, type FormEvent } from "react"
import { Cancel01StrokeRoundedIcon as X } from "@/shared/lib/icons"
import type {
  Appointment,
  AppointmentColor,
  AppointmentInput,
} from "@/entities/appointment"
import type {TimeStepMinutes} from "@/entities/calendar-settings"
import { formatDateTimeLocal, LUNCH_END_HOUR, LUNCH_START_HOUR, WORKDAY_END_HOUR, WORKDAY_START_HOUR } from "@/shared/lib/calendar-date"
import { Button } from "@/shared/ui/shadcn/button"
import { Input } from "@/shared/ui/shadcn/input"
import { Label } from "@/shared/ui/shadcn/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/shadcn/select"
import { Dialog, DialogContent } from "@/shared/ui/shadcn/dialog"
import { Popover, PopoverContent, PopoverPortal, PopoverPositioner } from "@/shared/ui/shadcn/popover"
import { Textarea } from "@/shared/ui/shadcn/textarea"

type AppointmentDialogProps = {
  open: boolean
  appointment?: Appointment
  appointments: Appointment[]
  timeStepMinutes: TimeStepMinutes
  initialStartAt: string
  initialEndAt?: string
  anchor?: DOMRect
  onClose: () => void
  onSave: (appointment: AppointmentInput, id?: string) => void
  onDelete: (id: string) => void
}

const colorOptions: { value: AppointmentColor; label: string }[] = [
  { value: "blue", label: "Синий" },
  { value: "teal", label: "Бирюзовый" },
  { value: "orange", label: "Оранжевый" },
  { value: "purple", label: "Фиолетовый" },
]

export function AppointmentDialog({
  open,
  appointment,
  appointments,
  timeStepMinutes,
  initialStartAt,
  initialEndAt,
  anchor,
  onClose,
  onSave,
  onDelete,
}: AppointmentDialogProps) {
  const [title, setTitle] = useState(appointment?.title ?? "")
  const [date, setDate] = useState(
    appointment?.startAt.slice(0, 10) ?? initialStartAt.slice(0, 10),
  )
  const [startTime, setStartTime] = useState(
    appointment?.startAt.slice(11, 16) ?? initialStartAt.slice(11, 16),
  )
  const [endDate, setEndDate] = useState(appointment?.endAt.slice(0, 10) ?? initialStartAt.slice(0, 10))
  const [endTime, setEndTime] = useState(appointment?.endAt.slice(11, 16) ?? initialStartAt.slice(11, 16))
  const [description, setDescription] = useState(appointment?.description ?? "")
  const [color, setColor] = useState<AppointmentColor>(appointment?.color ?? "blue")
  const [error, setError] = useState("")

  useEffect(() => {
    if (!open) return
    const defaultEnd = new Date(initialStartAt)
    defaultEnd.setMinutes(defaultEnd.getMinutes() + Math.ceil(50 / timeStepMinutes) * timeStepMinutes)
    const defaultEndAt = initialEndAt ?? formatDateTimeLocal(defaultEnd)
    setTitle(appointment?.title ?? "")
    setDate(appointment?.startAt.slice(0, 10) ?? initialStartAt.slice(0, 10))
    setStartTime(appointment?.startAt.slice(11, 16) ?? initialStartAt.slice(11, 16))
    setEndDate(appointment?.endAt.slice(0, 10) ?? defaultEndAt.slice(0, 10))
    setEndTime(appointment?.endAt.slice(11, 16) ?? defaultEndAt.slice(11, 16))
    setDescription(appointment?.description ?? "")
    setColor(appointment?.color ?? "blue")
    setError("")
  }, [appointment, initialEndAt, initialStartAt, open, timeStepMinutes])

  const handleClose = () => {
    onClose()
  }

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (new Date(`${endDate}T${endTime}`) <= new Date(`${date}T${startTime}`)) {
      setError("Время окончания должно быть позже времени начала.")
      return
    }
    const startMinutes = Number(startTime.slice(0, 2)) * 60 + Number(startTime.slice(3, 5))
    const endMinutes = Number(endTime.slice(0, 2)) * 60 + Number(endTime.slice(3, 5))
    const startTimeChanged = !appointment || appointment.startAt.slice(11, 16) !== startTime
    const endTimeChanged = !appointment || appointment.endAt.slice(11, 16) !== endTime
    if ((startTimeChanged && startMinutes % timeStepMinutes !== 0) || (endTimeChanged && endMinutes % timeStepMinutes !== 0)) {
      setError("Выберите время, кратное шагу прилипания из настроек календаря.")
      return
    }
    if (endDate !== date || startMinutes < WORKDAY_START_HOUR * 60 || endMinutes > WORKDAY_END_HOUR * 60 || (startMinutes < LUNCH_END_HOUR * 60 && endMinutes > LUNCH_START_HOUR * 60)) {
      setError("Запись должна быть в рабочее время 09:00–16:00 и не пересекать обед 13:00–14:00.")
      return
    }
    const start = new Date(`${date}T${startTime}`)
    const end = new Date(`${endDate}T${endTime}`)
    const hasConflict = appointments.some((item) => {
      if (item.id === appointment?.id || item.status === "cancelled") return false
      return start < new Date(item.endAt) && end > new Date(item.startAt)
    })
    if (hasConflict) {
      setError("Это время пересекается с другой записью. Выберите свободный интервал.")
      return
    }
    if (!title.trim()) {
      setError("Введите название записи.")
      return
    }

    handleClose()
    onSave(
      {
        title: title.trim(),
        startAt: `${date}T${startTime}`,
        endAt: `${endDate}T${endTime}`,
        description: description.trim(),
        color,
        status: "scheduled",
      },
      appointment?.id,
    )
  }

  const content = (
      <section>
        <div className="mb-6 flex items-start justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-blue-600">Запись в расписании</p>
            <h2 className="mt-1 text-xl font-semibold tracking-tight text-slate-900" id="appointment-dialog-title">
              {appointment?.status === "cancelled" ? "Восстановить запись" : appointment ? "Редактировать запись" : "Новая запись"}
            </h2>
          </div>
          <Button
            aria-label="Закрыть окно"
            className="rounded-lg text-slate-500 hover:text-slate-900"
            onClick={handleClose}
            size="icon"
            type="button"
            variant="ghost"
          >
            <X className="size-4" />
          </Button>
        </div>

        <form className="space-y-4" onSubmit={handleSubmit}>
          <Label className="block space-y-1.5 text-sm font-medium text-slate-700">
            Название
            <Input
              autoFocus
              className="h-11 w-full rounded-xl border border-slate-200 bg-white px-3 text-base font-normal outline-none transition focus:border-blue-400 focus:ring-4 focus:ring-blue-100"
              onChange={(event) => setTitle(event.target.value)}
              maxLength={120}
              placeholder="Например, планёрка"
              required
              value={title}
            />
          </Label>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Label className="block space-y-1.5 text-sm font-medium text-slate-700 sm:col-span-3">
              Дата
              <Input
                className="h-11 w-full rounded-xl border border-slate-200 bg-white px-3 text-base font-normal outline-none transition focus:border-blue-400 focus:ring-4 focus:ring-blue-100"
                onChange={(event) => {
                if (endDate === date) setEndDate(event.target.value)
                setDate(event.target.value)
                }}
                required
                type="date"
                value={date}
              />
            </Label>
            <Label className="block space-y-1.5 text-sm font-medium text-slate-700">
              Начало
              <Input
                className="h-11 w-full rounded-xl border border-slate-200 bg-white px-3 text-base font-normal outline-none transition focus:border-blue-400 focus:ring-4 focus:ring-blue-100"
                onChange={(event) => setStartTime(event.target.value)}
                required
                step={timeStepMinutes * 60}
                type="time"
                value={startTime}
              />
            </Label>
            <Label className="block space-y-1.5 text-sm font-medium text-slate-700">
              Окончание
              <Input
                className="h-11 w-full rounded-xl border border-slate-200 bg-white px-3 text-base font-normal outline-none transition focus:border-blue-400 focus:ring-4 focus:ring-blue-100"
                onChange={(event) => setEndTime(event.target.value)}
                required
                step={timeStepMinutes * 60}
                type="time"
                value={endTime}
              />
            </Label>
            <Label className="block space-y-1.5 text-sm font-medium text-slate-700 sm:col-span-3">
              Дата окончания
              <Input
                className="h-11 w-full rounded-xl border border-slate-200 bg-white px-3 text-base font-normal outline-none transition focus:border-blue-400 focus:ring-4 focus:ring-blue-100"
                min={date}
                onChange={(event) => setEndDate(event.target.value)}
                required
                type="date"
                value={endDate}
              />
            </Label>
            <Label className="block space-y-1.5 text-sm font-medium text-slate-700">
              Цвет
              <Select onValueChange={(value) => setColor(value as AppointmentColor)} value={color}>
                <SelectTrigger aria-label="Цвет записи" className="h-11 w-full rounded-xl bg-white px-3 text-base"><SelectValue>{colorOptions.find((option) => option.value === color)?.label}</SelectValue></SelectTrigger>
                <SelectContent>{colorOptions.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}</SelectContent>
              </Select>
            </Label>
          </div>

          <Label className="block space-y-1.5 text-sm font-medium text-slate-700">
            Заметки
            <Textarea
              className="min-h-24 w-full resize-y rounded-xl border border-slate-200 bg-white p-3 text-base font-normal outline-none transition focus:border-blue-400 focus:ring-4 focus:ring-blue-100"
              onChange={(event) => setDescription(event.target.value)}
              maxLength={1000}
              placeholder="Заметки без персональных данных"
              value={description}
            />
          </Label>

          {error && <p className="text-sm text-red-600">{error}</p>}

          <div className="flex items-center justify-between gap-3 border-t border-slate-100 pt-4">
            {appointment ? (
              <Button
                className="rounded-xl px-3 py-2 text-sm font-medium text-red-600 transition hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50"
                disabled={appointment.status === "cancelled"}
                onClick={() => {
                  handleClose()
                  onDelete(appointment.id)
                }}
                type="button"
                variant="ghost"
              >
                {appointment.status === "cancelled" ? "Запись отменена" : "Отменить запись"}
              </Button>
            ) : (
              <span />
            )}
            <div className="flex gap-2">
              <Button
                className="rounded-xl px-4 py-2.5 text-sm font-medium text-slate-600 transition hover:bg-slate-100"
                onClick={handleClose}
                type="button"
                variant="ghost"
              >
                Отмена
              </Button>
              <Button
                className="rounded-xl bg-blue-600 px-4 py-2.5 text-sm font-medium text-white shadow-sm transition hover:bg-blue-700"
                type="submit"
              >
                Сохранить
              </Button>
            </div>
          </div>
        </form>
      </section>
  )

  if (anchor && !appointment) {
    return (
      <Popover modal={false} onOpenChange={(nextOpen) => !nextOpen && handleClose()} open={open}>
        <PopoverPortal>
          <PopoverPositioner
            align="start"
            anchor={{ getBoundingClientRect: () => anchor }}
            collisionPadding={16}
            side="right"
            sideOffset={8}
          >
            <PopoverContent aria-labelledby="appointment-dialog-title" className="text-left">
              {content}
            </PopoverContent>
          </PopoverPositioner>
        </PopoverPortal>
      </Popover>
    )
  }

  return (
    <Dialog onOpenChange={(nextOpen) => !nextOpen && handleClose()} open={open}>
      <DialogContent aria-labelledby="appointment-dialog-title" className="max-h-[calc(100dvh-2rem)] overflow-y-auto p-6 text-left sm:max-w-lg" showCloseButton={false}>
        {content}
      </DialogContent>
    </Dialog>
  )
}
