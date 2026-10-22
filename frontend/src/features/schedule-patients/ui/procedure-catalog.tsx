"use client"

import {useState} from "react"
import type {Procedure} from "@/entities/procedure"
import {Cancel01StrokeRoundedIcon as X} from "@/shared/lib/icons"
import {usePageDrivenScroll} from "@/shared/lib/hooks/use-page-driven-scroll"
import {Button} from "@/shared/ui/shadcn/button"
import {Input} from "@/shared/ui/shadcn/input"
import {Label} from "@/shared/ui/shadcn/label"
import {Switch} from "@/shared/ui/shadcn/switch"
import {Tooltip, TooltipContent, TooltipProvider, TooltipTrigger} from "@/shared/ui/shadcn/tooltip"

const NUMBER_LIMITS = {
  durationMinutes: {min: 1, max: 420},
  preparationMinutes: {min: 0, max: 419},
  minIntervalDays: {min: 1, max: 365},
  defaultWeeklyCount: {min: 1, max: 7},
} as const

const catalogColumns = "xl:grid-cols-[minmax(12rem,1fr)_9rem_10rem_10rem_9rem_11rem_2.25rem]"

const COLUMN_HELP = {
  duration: {
    label: "Процедура, мин",
    description: "Время самой процедуры без переодевания и подготовки. По умолчанию 30 минут.",
  },
  preparation: {
    label: "Подготовка, мин",
    description: "Суммарное время перед и после процедуры, например 10 минут до и 10 минут после. По умолчанию 20 минут. Планировщик резервирует время процедуры плюс подготовку.",
  },
  interval: {
    label: "Интервал, дн.",
    description: "Минимальное число календарных дней между сеансами одной процедуры у одного пациента (от 1 до 365). Например, при значении 2 следующий сеанс возможен через два дня.",
  },
  weekly: {
    label: "В неделю",
    description: "Рекомендуемое количество сеансов за 7 дней (от 1 до 7). По нему рассчитывается начальное число сеансов на весь срок лечения; в плане пациента его можно изменить.",
  },
  relaxing: {
    label: "Расслабляющая",
    description: "Признак расслабляющей процедуры. При составлении курса для пациентов с высоким приоритетом такие процедуры размещаются раньше остальных и выделяются бирюзовым цветом в календаре.",
  },
} as const

type HelpField = keyof typeof COLUMN_HELP

function ProcedureColumnHelp({field}: {field: HelpField}) {
  const {label, description} = COLUMN_HELP[field]

  return (
    <Tooltip>
      <TooltipTrigger
        aria-label={`Пояснение: ${label}`}
        className="inline-flex min-h-7 w-fit items-center gap-1.5 whitespace-nowrap text-left text-sm font-medium text-slate-600 outline-none focus-visible:rounded-sm focus-visible:ring-2 focus-visible:ring-blue-500"
        type="button"
      >
        <span>{label}</span>
        <span aria-hidden="true" className="inline-flex size-4 shrink-0 items-center justify-center rounded-full border border-slate-300 text-[10px] leading-none text-slate-500">?</span>
      </TooltipTrigger>
      <TooltipContent align="start" className="max-w-[min(18rem,calc(100vw-2rem))] text-left leading-relaxed" side="bottom" sideOffset={6}>
        {description}
      </TooltipContent>
    </Tooltip>
  )
}

type NumberField = keyof typeof NUMBER_LIMITS

type ProcedureCatalogProps = {
  procedures: Procedure[]
  onUpdateProcedureAction: (id: string, changes: Partial<Procedure>) => void
  onRemoveProcedureAction: (id: string) => void
}

export const ProcedureCatalog = (props: ProcedureCatalogProps) => {
  const {procedures, onUpdateProcedureAction, onRemoveProcedureAction} = props
  const [numberDrafts, setNumberDrafts] = useState<Record<string, string>>({})
  const {trackRef, panelRef, viewportRef, contentRef, panelStyle} = usePageDrivenScroll({
    topOffset: 64,
    bottomGap: 16,
    heightMode: "fit-viewport",
  })

  const getDraftKey = (procedureId: string, field: NumberField) => `${procedureId}:${field}`
  const getNumberValue = (procedure: Procedure, field: NumberField) =>
    numberDrafts[getDraftKey(procedure.id, field)] ?? procedure[field] ?? 1

  const clearNumberDraft = (procedureId: string, field: NumberField) => {
    const key = getDraftKey(procedureId, field)
    setNumberDrafts((previous) => {
      if (!(key in previous)) return previous
      const next = {...previous}
      delete next[key]
      return next
    })
  }

  const updateNumber = (procedureId: string, field: NumberField, rawValue: string) => {
    const value = Number(rawValue)
    const {min, max} = NUMBER_LIMITS[field]
    const selectedProcedure = procedures.find((item) => item.id === procedureId)
    const combinedMinutes = field === "durationMinutes" ? value + (selectedProcedure?.preparationMinutes ?? 0) : field === "preparationMinutes" ? value + (selectedProcedure?.durationMinutes ?? 0) : 0
    const isValid = /^\d+$/.test(rawValue) && Number.isSafeInteger(value) && value >= min && value <= max && (!combinedMinutes || combinedMinutes <= 420)

    if (!isValid) {
      setNumberDrafts((previous) => ({...previous, [getDraftKey(procedureId, field)]: rawValue}))
      return
    }

    clearNumberDraft(procedureId, field)
    onUpdateProcedureAction(procedureId, {[field]: value})
  }

  return (
    <TooltipProvider delay={250}>
    <section aria-label="Каталог процедур" className="relative min-w-0 shrink-0" ref={trackRef}>
      <section className="sticky flex min-h-0 flex-col overflow-hidden rounded-[20px] bg-white" ref={panelRef} style={panelStyle}>
        {procedures.length > 0 && <div className={`z-10 hidden shrink-0 gap-3 border-b border-slate-200 px-3 py-3 font-medium text-slate-600 xl:grid ${catalogColumns}`}>
          <span>Название процедуры</span>
          <ProcedureColumnHelp field="duration" />
          <ProcedureColumnHelp field="preparation" />
          <ProcedureColumnHelp field="interval" />
          <ProcedureColumnHelp field="weekly" />
          <ProcedureColumnHelp field="relaxing" />
          <span className="sr-only">Удалить</span>
        </div>}
        {procedures.length > 0 && (
          <div aria-label="Пояснения к параметрам процедур" className="grid shrink-0 grid-cols-2 gap-x-4 gap-y-1 border-b border-slate-200 px-3 py-2 sm:grid-cols-5 xl:hidden">
            <ProcedureColumnHelp field="duration" />
            <ProcedureColumnHelp field="preparation" />
            <ProcedureColumnHelp field="interval" />
            <ProcedureColumnHelp field="weekly" />
            <ProcedureColumnHelp field="relaxing" />
          </div>
        )}
        <div className="min-h-0 flex-1 overflow-hidden" ref={viewportRef}>
          <div className="divide-y divide-slate-100" ref={contentRef}>
        {procedures.length ? procedures.map((procedure) => <article className={`grid gap-3 p-3 transition-colors focus-within:bg-blue-50/30 hover:bg-slate-50 sm:grid-cols-[minmax(0,1fr)_minmax(9rem,0.8fr)_minmax(9rem,0.8fr)] sm:items-center ${catalogColumns}`} key={procedure.id}>
          <Input aria-label="Название процедуры" className="h-10 min-w-0 rounded-lg px-3 text-sm" maxLength={480} onChange={(event) => onUpdateProcedureAction(procedure.id, {name: event.target.value})} value={procedure.name}/>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="shrink-0 xl:hidden">Процедура, мин</span>
            <Input aria-invalid={getDraftKey(procedure.id, "durationMinutes") in numberDrafts} aria-label={`Продолжительность процедуры ${procedure.name}, минут`} className="h-9 w-full min-w-0 flex-1 rounded-lg px-2 text-sm text-slate-900" max={420} min={1} onBlur={() => clearNumberDraft(procedure.id, "durationMinutes")} onChange={(event) => updateNumber(procedure.id, "durationMinutes", event.target.value)} step={1} title="От 1 до 420 минут, вместе с подготовкой не более 420" type="number" value={getNumberValue(procedure, "durationMinutes")}/>
          </Label>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="shrink-0 xl:hidden">Подготовка, мин</span>
            <Input aria-invalid={getDraftKey(procedure.id, "preparationMinutes") in numberDrafts} aria-label={`Время подготовки к процедуре ${procedure.name}, минут`} className="h-9 w-full min-w-0 flex-1 rounded-lg px-2 text-sm text-slate-900" max={419} min={0} onBlur={() => clearNumberDraft(procedure.id, "preparationMinutes")} onChange={(event) => updateNumber(procedure.id, "preparationMinutes", event.target.value)} step={1} title={`Всего в расписании: ${procedure.durationMinutes + procedure.preparationMinutes} мин`} type="number" value={getNumberValue(procedure, "preparationMinutes")}/>
          </Label>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="shrink-0 xl:hidden">Интервал, дн.</span>
            <Input aria-invalid={getDraftKey(procedure.id, "minIntervalDays") in numberDrafts} aria-label={`Минимальный интервал между процедурами ${procedure.name}, дней`} className="h-9 w-full min-w-0 flex-1 rounded-lg px-2 text-sm text-slate-900" max={365} min={1} onBlur={() => clearNumberDraft(procedure.id, "minIntervalDays")} onChange={(event) => updateNumber(procedure.id, "minIntervalDays", event.target.value)} step={1} title="От 1 до 365 дней" type="number" value={getNumberValue(procedure, "minIntervalDays")}/>
          </Label>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="shrink-0 xl:hidden">В неделю</span>
            <Input aria-invalid={getDraftKey(procedure.id, "defaultWeeklyCount") in numberDrafts} aria-label={`Количество процедур ${procedure.name} в неделю по умолчанию`} className="h-9 w-full min-w-0 flex-1 rounded-lg px-2 text-sm text-slate-900" max={7} min={1} onBlur={() => clearNumberDraft(procedure.id, "defaultWeeklyCount")} onChange={(event) => updateNumber(procedure.id, "defaultWeeklyCount", event.target.value)} step={1} title="От 1 до 7 процедур в неделю" type="number" value={getNumberValue(procedure, "defaultWeeklyCount")}/>
          </Label>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-600"><Switch aria-label={`Расслабляющая процедура: ${procedure.name}`} checked={procedure.isRelaxing} onCheckedChange={(checked) => onUpdateProcedureAction(procedure.id, {isRelaxing: checked})}/><span className="xl:hidden">Расслабляющая процедура</span></Label>
          <Button aria-label={`Удалить ${procedure.name}`} className="size-9 justify-self-end rounded-lg text-slate-400 hover:bg-red-50 hover:text-red-600 sm:justify-self-center" onClick={() => onRemoveProcedureAction(procedure.id)} size="icon" type="button" variant="ghost"><X className="size-4"/></Button>
        </article>) : <p className="p-6 text-center text-sm text-slate-500">Каталог пуст. Добавьте процедуру, чтобы продолжить.</p>}
          </div>
        </div>
      </section>
    </section>
    </TooltipProvider>
  )
}
