"use client"

import {useEffect, type PointerEvent as ReactPointerEvent} from "react"
import {createPortal} from "react-dom"
import type {Appointment} from "@/entities/appointment"
import {Button} from "@/shared/ui/shadcn/button"

export type AppointmentCluster = {
  day: string
  appointments: Appointment[]
  left: number
  top: number
}

type Props = {
  cluster: AppointmentCluster | null
  onClose: () => void
  onEdit: (appointment: Appointment) => void
  onStartDrag: (event: ReactPointerEvent<HTMLButtonElement>, appointment: Appointment) => void
  onCreate: (appointment: Appointment) => void
  onAssignProcedure: (appointment: Appointment, day: string) => void
}

const timeLabel = (date: string) => date.slice(11, 16)

export const AppointmentClusterPanel = ({
  cluster, onClose, onEdit, onStartDrag, onCreate, onAssignProcedure,
}: Props) => {
  useEffect(() => {
    if (!cluster) return
    const handlePointer = (event: PointerEvent) => {
      if (event.target instanceof Element && event.target.closest("[data-appointment-cluster-panel], [data-appointment-cluster]")) return
      onClose()
    }
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose()
    }
    document.addEventListener("pointerdown", handlePointer)
    document.addEventListener("keydown", handleKey)
    return () => {
      document.removeEventListener("pointerdown", handlePointer)
      document.removeEventListener("keydown", handleKey)
    }
  }, [cluster, onClose])

  if (!cluster || typeof document === "undefined") return null

  const first = cluster.appointments[0]
  const patientNumbers = [...new Set(cluster.appointments.flatMap((item) => item.patientNumber ? [item.patientNumber] : []))]
  const courseAppointment = patientNumbers.length === 1
    ? cluster.appointments.find((item) => item.patientNumber === patientNumbers[0]) ?? first
    : {...first, patientNumber: undefined, sourcePlanDate: undefined}
  return createPortal(
    <section
      aria-label="Сеансы в группе"
      className="fixed z-[60] flex w-[min(21rem,calc(100vw-2rem))] max-h-[min(35rem,80dvh)] flex-col overflow-hidden rounded-xl border border-slate-200 bg-white p-3 text-slate-900 shadow-xl"
      data-appointment-cluster-panel
      role="dialog"
      style={{left: cluster.left, top: cluster.top}}
    >
      <div className="mb-2 flex items-start justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold">{cluster.appointments.length} сеансов в группе</h3>
          <p className="text-xs text-slate-600">Потяните «Перетащить» на свободное время календаря.</p>
        </div>
        <Button aria-label="Закрыть список" className="size-7 shrink-0 rounded-md" onClick={onClose} size="icon" type="button" variant="ghost">×</Button>
      </div>
      <div className="min-h-0 space-y-2 overflow-y-auto">
        {cluster.appointments.map((appointment) => (
          <div className="flex items-center gap-2 rounded-lg border border-slate-200 p-2" key={appointment.id}>
            <button
              className="min-w-0 flex-1 cursor-pointer text-left hover:text-blue-700 focus-visible:rounded focus-visible:outline-2 focus-visible:outline-blue-500"
              onClick={() => onEdit(appointment)}
              type="button"
            >
              <span className="block truncate text-sm font-medium">{appointment.title}</span>
              <span className="text-xs text-slate-500">
                {timeLabel(appointment.startAt)}–{timeLabel(appointment.endAt)}
                {appointment.room ? ` · Каб. ${appointment.room}` : ""}
              </span>
            </button>
            {appointment.status !== "cancelled" && (
              <Button
                aria-label={`Перетащить ${appointment.title}`}
                className="h-8 shrink-0 cursor-grab touch-none rounded-md px-2 text-xs active:cursor-grabbing"
                onPointerDown={(event) => onStartDrag(event, appointment)}
                title="Удерживайте и перетащите на нужный день и время"
                type="button"
                variant="outline"
              >Перетащить</Button>
            )}
          </div>
        ))}
      </div>
      <div className="mt-3 grid gap-2 border-t border-slate-100 pt-3">
        <Button className="h-9 rounded-lg text-xs" onClick={() => onAssignProcedure(courseAppointment, cluster.day)} type="button" variant="outline">
          {patientNumbers.length === 1 ? `Назначить процедуру пациенту ${patientNumbers[0]}` : "Открыть назначения пациентов"}
        </Button>
        <Button className="h-9 rounded-lg text-xs" onClick={() => onCreate(first)} type="button" variant="ghost">
          Новая отдельная запись
        </Button>
      </div>
    </section>,
    document.body,
  )
}
