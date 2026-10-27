"use client"

import {useState} from "react"
import {ru} from "date-fns/locale"
import {Calendar03StrokeRoundedIcon} from "@/shared/lib/icons"
import {formatDateKey, parseDateKey} from "@/shared/lib/date-range"
import {cn} from "@/shared/lib/utils"
import {Button} from "@/shared/ui/shadcn/button"
import {Calendar} from "@/shared/ui/shadcn/calendar"
import {Popover, PopoverContent, PopoverPositioner, PopoverPortal, PopoverTrigger} from "@/shared/ui/shadcn/popover"

const dateFormatter = new Intl.DateTimeFormat("ru-RU", {day: "2-digit", month: "2-digit", year: "numeric"})

type DatePickerProps = {
  dateKey: string
  ariaLabel: string
  disabled?: boolean
  minDateKey?: string
  maxDateKey?: string
  className?: string
  onChange: (dateKey: string) => void
}

function DatePicker({dateKey, ariaLabel, disabled = false, minDateKey, maxDateKey, className, onChange}: DatePickerProps) {
  const [open, setOpen] = useState(false)
  const selectedDate = parseDateKey(dateKey)
  const minDate = minDateKey ? parseDateKey(minDateKey) : undefined
  const maxDate = maxDateKey ? parseDateKey(maxDateKey) : undefined
  const disabledDays = [
    ...(minDate ? [{before: minDate}] : []),
    ...(maxDate ? [{after: maxDate}] : []),
  ]

  const selectDate = (date: Date | undefined) => {
    if (!date) return
    const nextDateKey = formatDateKey(date)
    if ((minDateKey && nextDateKey < minDateKey) || (maxDateKey && nextDateKey > maxDateKey)) return
    setOpen(false)
    if (nextDateKey !== dateKey) onChange(nextDateKey)
  }

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger render={<Button aria-label={ariaLabel} className={cn("h-10 w-full justify-between rounded-xl bg-white px-3 text-left font-normal", className)} disabled={disabled} type="button" variant="outline"/>}>
        <span>{selectedDate ? dateFormatter.format(selectedDate) : dateKey}</span>
        <Calendar03StrokeRoundedIcon className="size-4 text-slate-500"/>
      </PopoverTrigger>
      <PopoverPortal>
        <PopoverPositioner align="start">
          <PopoverContent className="!w-auto !max-w-[calc(100vw-1rem)] !p-3">
            <Calendar
              mode="single"
              selected={selectedDate}
              onSelect={selectDate}
              defaultMonth={selectedDate ?? minDate}
              numberOfMonths={1}
              locale={ru}
              showOutsideDays
              disabled={disabledDays.length ? disabledDays : undefined}
            />
          </PopoverContent>
        </PopoverPositioner>
      </PopoverPortal>
    </Popover>
  )
}

export {DatePicker}
