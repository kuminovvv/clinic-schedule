"use client"

import {useEffect, useState} from "react"
import {getClientData, sendBackendRequest} from "@/shared/api/backend-client"
import {notify} from "@/shared/lib/notifications"
import {Button} from "@/shared/ui/shadcn/button"
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from "@/shared/ui/shadcn/dialog"
import {Label} from "@/shared/ui/shadcn/label"
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from "@/shared/ui/shadcn/select"
import {DatePicker} from "@/shared/ui/shadcn/date-picker"

type AbsenceReason = "illness" | "other"
type Absence = {date: string; patientNumber: number; from: string; to: string; reason: string}
type AbsenceResult = {
  affectedCount: number
  rescheduledCount: number
  unscheduledCount: number
  unavailableCount: number
  warnings?: string[]
  appointments?: unknown[]
}

const asUtc = (day: string) => new Date(`${day}T00:00:00Z`)
const displayDay = (day: string) => asUtc(day).toLocaleDateString("ru-RU", {day: "numeric", month: "short", timeZone: "UTC"})

export function PatientAbsenceControl({courseKey, patientNumber, arrivalDate, departureDate}: {courseKey: string; patientNumber: number; arrivalDate: string; departureDate: string}) {
  const [open, setOpen] = useState(false)
  const [from, setFrom] = useState(arrivalDate)
  const [to, setTo] = useState(arrivalDate)
  const [reason, setReason] = useState<AbsenceReason>("illness")
  const [preview, setPreview] = useState<AbsenceResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [absences, setAbsences] = useState<Absence[]>([])
  const valid = from >= arrivalDate && from <= to && to <= departureDate

  const loadAbsences = async () => {
    try {
      const result = await getClientData<Absence[] | {absences: Absence[]}>(`/v1/patient-absences?date=${courseKey}&patientNumber=${patientNumber}`)
      setAbsences(Array.isArray(result) ? result : result.absences ?? [])
    } catch {
      // Error interceptor supplies a notice. The absence action remains available.
    }
  }

  useEffect(() => {
    let active = true
    const load = async () => {
      try {
        const result = await getClientData<Absence[] | {absences: Absence[]}>(`/v1/patient-absences?date=${courseKey}&patientNumber=${patientNumber}`)
        if (active) setAbsences(Array.isArray(result) ? result : result.absences ?? [])
      } catch {
        // The interceptor explains API errors; no automatic retries.
      }
    }
    void load()
    return () => {active = false}
  }, [courseKey, patientNumber])

  const reset = () => {
    setPreview(null)
    setFrom(arrivalDate)
    setTo(arrivalDate)
    setReason("illness")
  }

  const submit = async () => {
    if (busy) return
    if (!valid) {
      notify.warning("Даты вне периода лечения", `Выберите даты с ${displayDay(arrivalDate)} по ${displayDay(departureDate)}.`)
      return
    }
    setBusy(true)
    try {
      const result = await sendBackendRequest<AbsenceResult>("/v1/patient-absences", "POST", {
        date: courseKey, patientNumber, from, to, reason, preview: preview === null,
      })
      if (!result) throw new Error("Не удалось получить результат перепланирования")
      if (preview === null) {
        setPreview(result)
        if (result.unscheduledCount > 0) notify.warning("Не все сеансы можно перенести", `Не помещаются в период лечения: ${result.unscheduledCount}. Измените даты лечения или отсутствия.`)
        if (result.warnings?.length) notify.warning("Проверьте перенос", result.warnings.join("\n"))
      } else {
        notify.success("Отсутствие учтено", `Перенесено сеансов: ${result.rescheduledCount}. Остальные пациенты и ручные изменения сохранены.`)
        setOpen(false)
        reset()
        await loadAbsences()
      }
    } catch {
      // Failed operation leaves the editor intact and makes no optimistic changes.
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-slate-200 px-3 py-2.5 text-sm">
        <div>
          <p className="font-medium text-slate-800">Отсутствие пациента</p>
          <p className="text-xs text-slate-500">Болезнь или другая причина. Переносим только затронутые сеансы.</p>
          {absences.length > 0 && <p className="mt-1 text-xs text-slate-600">Уже отмечено: {absences.map((item) => `${displayDay(item.from)}–${displayDay(item.to)}`).join(", ")}</p>}
        </div>
        <Button size="sm" type="button" variant="outline" onClick={() => {reset(); setOpen(true)}}>Указать даты</Button>
      </div>
      <Dialog open={open} onOpenChange={(next) => {if (!busy) {setOpen(next); if (!next) reset()}}}>
        <DialogContent className="sm:max-w-lg" showCloseButton={!busy}>
          <DialogHeader>
            <DialogTitle>Пациент {patientNumber} отсутствует</DialogTitle>
            <DialogDescription>Выберите даты отсутствия. Сначала покажем, сколько сеансов потребуется перенести. Остальные пациенты и изменения, внесённые вручную, не затрагиваются.</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <Label className="grid gap-1.5 text-sm">Причина
              <Select value={reason} disabled={busy || preview !== null} onValueChange={(value) => {if (value === "illness" || value === "other") setReason(value)}}>
                <SelectTrigger className="h-10 w-full"><SelectValue>{reason === "illness" ? "Заболел" : "Другая причина"}</SelectValue></SelectTrigger>
                <SelectContent><SelectItem value="illness">Заболел</SelectItem><SelectItem value="other">Другая причина</SelectItem></SelectContent>
              </Select>
            </Label>
            <div className="grid grid-cols-2 gap-3">
              <Label className="grid gap-1.5 text-sm">Отсутствует с
                <DatePicker ariaLabel="Отсутствует с" className="h-10" dateKey={from} minDateKey={arrivalDate} maxDateKey={departureDate} disabled={busy || preview !== null} onChange={(next) => {setFrom(next);if(to<next)setTo(next)}}/>
              </Label>
              <Label className="grid gap-1.5 text-sm">По (включительно)
                <DatePicker ariaLabel="Отсутствует по (включительно)" className="h-10" dateKey={to} minDateKey={from} maxDateKey={departureDate} disabled={busy || preview !== null} onChange={setTo}/>
              </Label>
            </div>
            {preview && (
              <div className="space-y-1 rounded-lg bg-slate-50 p-3 text-sm">
                <p className="font-medium">Предварительный результат</p>
                <p>Затронуто сеансов: {preview.affectedCount}. Планируется перенести: {preview.rescheduledCount}.</p>
                {preview.unscheduledCount === 0 && <p className="text-xs text-slate-500">Подтвердите, чтобы сохранить отсутствие и обновить расписание.</p>}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" type="button" disabled={busy} onClick={() => {setOpen(false);reset()}}>Отмена</Button>
            {preview && <Button variant="outline" type="button" disabled={busy} onClick={() => setPreview(null)}>Изменить даты</Button>}
            <Button type="button" disabled={busy || !valid || (preview !== null && preview.unscheduledCount > 0)} onClick={() => void submit()}>{busy ? "Проверка…" : preview ? "Подтвердить перенос" : "Проверить перенос"}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
