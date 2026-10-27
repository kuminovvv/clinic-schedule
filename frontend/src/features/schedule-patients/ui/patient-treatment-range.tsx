"use client"

import {useMemo, useState} from "react"
import {ru} from "date-fns/locale"
import {type DateRange} from "react-day-picker"
import {Calendar03StrokeRoundedIcon} from "@/shared/lib/icons"
import {Button} from "@/shared/ui/shadcn/button"
import {Calendar} from "@/shared/ui/shadcn/calendar"
import {Popover, PopoverContent, PopoverPositioner, PopoverPortal, PopoverTrigger} from "@/shared/ui/shadcn/popover"
import {addCalendarDays, formatDateKey, formatDateLabel, inclusiveDayCount, parseDateKey} from "@/shared/lib/date-range"

type PatientTreatmentRangeProps = {
  courseKey: string
  arrivalDate: string
  departureDate: string
  onChange: (arrivalDate: string, departureDate: string) => void
}

const toPickerRange = (arrivalDate: string, departureDate: string): DateRange | undefined => {
  const from = parseDateKey(arrivalDate)
  const to = parseDateKey(departureDate)
  return from && to ? {from, to} : undefined
}

export function PatientTreatmentRange({courseKey, arrivalDate, departureDate, onChange}: PatientTreatmentRangeProps) {
  const currentRange = useMemo(() => toPickerRange(arrivalDate, departureDate), [arrivalDate, departureDate])
  const firstAllowedDate = parseDateKey(courseKey)
  const lastAllowedDate = parseDateKey(addCalendarDays(courseKey, 364))
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<DateRange | undefined>()
  const draftDays = draft?.from && draft.to ? inclusiveDayCount(formatDateKey(draft.from), formatDateKey(draft.to)) : 0
  const validDraft = draftDays >= 1 && draftDays <= 365

  const openPicker = (next: boolean) => {
    if (next) setDraft(currentRange)
    setOpen(next)
  }

  const commitRange = (range: DateRange | undefined) => {
    if (!range?.from || !range.to) return
    const from = formatDateKey(range.from)
    const to = formatDateKey(range.to)
    if (!from || !to || to < from || inclusiveDayCount(from, to) > 365) return
    if (from < courseKey || to > addCalendarDays(courseKey, 364)) return
    onChange(from, to)
    setOpen(false)
  }

  return (
    <div>
      <div className="mb-2">
        <h3 className="text-sm font-semibold text-slate-900">Даты поступления и выезда</h3>
      </div>
      <Popover onOpenChange={openPicker} open={open} >
        <PopoverTrigger render={<Button aria-label="Выбрать даты поступления и выезда" className="h-9 w-full justify-start gap-2 rounded-lg bg-background px-3 text-left font-normal text-slate-700 sm:w-auto mb-2" type="button" variant="outline"/>}>
          <Calendar03StrokeRoundedIcon className="size-4 text-slate-500"/>
          <span>Поступление: {formatDateLabel(arrivalDate)} · выезд: {formatDateLabel(departureDate)}</span>
        </PopoverTrigger>
        <PopoverPortal>
          <PopoverPositioner align="start">
            <PopoverContent className="w-auto! max-w-[calc(100vw-1rem)]! p-3!">
              <Calendar
                mode="range"
                selected={draft}
                onSelect={setDraft}
                defaultMonth={draft?.from ?? currentRange?.from}
                numberOfMonths={1}
                locale={ru}
                showOutsideDays
                disabled={firstAllowedDate && lastAllowedDate ? [{before: firstAllowedDate}, {after: lastAllowedDate}] : undefined}
                className="[--rdp-accent-color:#2563eb] [--rdp-accent-background-color:#dbeafe]"
              />
              <div className="flex items-center justify-between gap-2 border-t border-slate-100 pt-2">
                <span className="text-xs text-slate-500">{draft?.from ? (draft.to ? `${formatDateLabel(formatDateKey(draft.from))} — ${formatDateLabel(formatDateKey(draft.to))}${!validDraft ? " · максимум 365 дней" : ""}` : `${formatDateLabel(formatDateKey(draft.from))} — выберите выезд`) : "Выберите поступление"}</span>
                <div className="flex shrink-0 gap-2">
                  {draft?.from && <Button className="h-8 rounded-md px-2 text-xs" onClick={() => commitRange({from: draft.from, to: draft.from})} type="button" variant="ghost">Один день</Button>}
                  <Button className="h-8 rounded-md px-3 text-xs" disabled={!validDraft} onClick={() => commitRange(draft)} type="button">Готово</Button>
                </div>
              </div>
            </PopoverContent>
          </PopoverPositioner>
        </PopoverPortal>
      </Popover>
    </div>
  )
}
