"use client"

import {useRef, useState} from "react"
import {Button} from "@/shared/ui/shadcn/button"
import {Input} from "@/shared/ui/shadcn/input"
import {Label} from "@/shared/ui/shadcn/label"

const dateLabel = new Intl.DateTimeFormat("ru-RU", {
  day: "numeric",
  month: "long",
  year: "numeric",
  timeZone: "UTC",
})

const calendarMonthDays = (dateKey: string) => {
  const [year, month, day] = dateKey.split("-").map(Number)
  const nextMonthLastDay = new Date(Date.UTC(year, month + 1, 0)).getUTCDate()
  const nextMonthDate = Date.UTC(year, month, Math.min(day, nextMonthLastDay))
  return Math.round((nextMonthDate - Date.UTC(year, month - 1, day)) / 86_400_000)
}

type StayDurationControlProps = {
  admissionDateKey: string
  stayDays: number
  onChange: (days: number) => void
}

export function StayDurationControl({admissionDateKey, stayDays, onChange}: StayDurationControlProps) {
  const [customMode, setCustomMode] = useState(false)
  const [draft, setDraft] = useState<string | null>(null)
  const customInput = useRef<HTMLInputElement>(null)

  const presets = [
    {label: "1 день", days: 1},
    {label: "1 неделя", days: 7},
    {label: "2 недели", days: 14},
    {label: "1 месяц", days: calendarMonthDays(admissionDateKey)},
  ]
  const hasPreset = presets.some((preset) => preset.days === stayDays)
  const showCustom = customMode || !hasPreset
  const rawValue = draft ?? String(stayDays)
  const customValid = /^\d+$/.test(rawValue) && Number.isSafeInteger(Number(rawValue)) && Number(rawValue) >= 1 && Number(rawValue) <= 365

  const choosePreset = (days: number) => {
    setCustomMode(false)
    setDraft(null)
    onChange(days)
  }

  const commitCustom = (value: string) => {
    const days = Number(value)
    if (!/^\d+$/.test(value) || !Number.isSafeInteger(days) || days < 1 || days > 365) return
    setDraft(null)
    if (days !== stayDays) onChange(days)
  }

  const admission = new Date(`${admissionDateKey}T00:00:00Z`)
  const lastDay = new Date(admission)
  lastDay.setUTCDate(lastDay.getUTCDate() + stayDays - 1)

  return (
    <section aria-labelledby="stay-duration-title" className="mb-3 rounded-xl bg-slate-50 px-3 py-3 sm:px-4">
      <div className="mb-3">
        <h3 className="text-sm font-semibold text-slate-900" id="stay-duration-title">Срок пребывания</h3>
        <p className="mt-1 text-xs text-slate-500">Выберите срок лечения. День поступления тоже учитывается.</p>
      </div>
      <div aria-label="Срок пребывания пациента" className="flex flex-wrap gap-1.5" role="group">
        {presets.map((preset) => {
          const active = !showCustom && stayDays === preset.days
          return (
            <Button
              key={preset.label}
              aria-pressed={active}
              className={`h-9 rounded-lg px-3 text-sm transition-colors ${active ? "bg-white text-blue-700 shadow-sm ring-1 ring-slate-200 hover:bg-white" : "text-slate-600 hover:bg-white hover:text-slate-900"}`}
              onClick={() => choosePreset(preset.days)}
              size="sm"
              type="button"
              variant="ghost"
            >
              {preset.label}
            </Button>
          )
        })}
        <Button
          aria-pressed={showCustom}
          className={`h-9 rounded-lg px-3 text-sm transition-colors ${showCustom ? "bg-white text-blue-700 shadow-sm ring-1 ring-slate-200 hover:bg-white" : "text-slate-600 hover:bg-white hover:text-slate-900"}`}
          onClick={() => {
            setCustomMode(true)
            requestAnimationFrame(() => customInput.current?.focus())
          }}
          size="sm"
          type="button"
          variant="ghost"
        >
          Другой срок
        </Button>
      </div>
      {showCustom && (
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Label htmlFor="patient-stay-days" className="text-sm text-slate-700">Количество дней</Label>
          <Input
            ref={customInput}
            id="patient-stay-days"
            aria-invalid={!customValid}
            aria-describedby={!customValid ? "stay-days-error" : undefined}
            className="h-9 w-24 rounded-lg bg-white px-3 text-sm tabular-nums"
            inputMode="numeric"
            min={1}
            max={365}
            step={1}
            type="number"
            value={rawValue}
            onChange={(event) => setDraft(event.target.value)}
            onBlur={(event) => commitCustom(event.currentTarget.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") event.currentTarget.blur()
              if (event.key === "Escape") {
                setDraft(null)
                event.currentTarget.value = String(stayDays)
                event.currentTarget.blur()
              }
            }}
          />
          {!customValid && <span className="text-xs text-red-700" id="stay-days-error" role="alert">От 1 до 365 дней</span>}
        </div>
      )}
      <p className="mt-3 text-xs text-slate-600" role="status">
        Период: {dateLabel.format(admission)} — {dateLabel.format(lastDay)} включительно
      </p>
    </section>
  )
}
