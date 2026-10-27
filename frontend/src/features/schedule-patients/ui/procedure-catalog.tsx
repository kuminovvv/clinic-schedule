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
import {CatalogPrerequisiteSelect} from "./catalog-prerequisite-select"

const NUMBER_LIMITS = {
  durationMinutes: {min: 1, max: 420},
  preparationMinutes: {min: 0, max: 419},
  minIntervalDays: {min: 0, max: 364},
  defaultWeeklyCount: {min: 1, max: 7},
  defaultSessionCount: {min: 1, max: 365},
} as const

// Compact enough for the page container at 1280px, without a forced
// horizontal scrollbar. Numeric fields need little more than 3 digits.
const catalogColumns = "xl:grid-cols-[minmax(9rem,1fr)_5.5rem_5.5rem_5rem_4.5rem_5rem_7rem_5.5rem_minmax(9rem,1fr)_2.25rem]"

const COLUMN_HELP = {
  duration: {
    label: "Время, мин",
    description: "Время самой процедуры без переодевания и подготовки. По умолчанию 30 минут.",
  },
  preparation: {
    label: "Подгот., мин",
    description: "Суммарное время перед и после процедуры, например 10 минут до и 10 минут после. По умолчанию 20 минут. Планировщик резервирует время процедуры плюс подготовку.",
  },
  interval: {
    label: "Перерыв, дн.",
    description: "Сколько полных дней должно пройти между сеансами одной процедуры у пациента. Значение 0 разрешает проводить её каждый день, но не дважды в один день.",
  },
  weekly: {
    label: "В неделю",
    description: "Максимальное количество сеансов за любые 7 последовательных дней (от 1 до 7). Этот предел соблюдается независимо от общего числа сеансов на курс.",
  },
  sessions: {
    label: "На курс",
    description: "Начальное число сеансов новой процедуры в плане пациента (от 1 до 365), но не больше длительности пребывания в днях. Не зависит от недельного предела. Уже назначенные пациентам процедуры не изменятся.",
  },
  relaxing: {
    label: "Расслабл.",
    description: "Признак расслабляющей процедуры. Для пациента высокой тяжести её можно назначить обязательной перед выбранными сложными процедурами. Сеансы размещаются раньше связанных сложных и выделяются бирюзовым цветом в календаре.",
  },
  prerequisite: {
    label: "Перед сеансом",
    description: "Для сложной процедуры можно выбрать расслабляющую подготовку. При назначении пациенту с высокой тяжестью она добавится автоматически и пройдёт в тот же день перед сложной процедурой. Для отдельного пациента правило можно изменить.",
  },
  room: {
    label: "Кабинет",
    description: "Необязательный кабинет процедуры, например 101 или Массажный. Если оставить пустым, процедура доступна по общему графику организации. При указании кабинета он не может быть занят двумя пациентами одновременно.",
  },
} as const

type HelpField = keyof typeof COLUMN_HELP

const ProcedureColumnHelp = ({field}: {field: HelpField}) => {
  const {label, description} = COLUMN_HELP[field]

  return (
    <Tooltip>
      <TooltipTrigger
        aria-label={`Пояснение: ${label}`}
        className="inline-flex min-h-7 min-w-0 max-w-full flex-wrap items-center gap-x-1 gap-y-0.5 text-left text-xs font-medium leading-tight text-slate-600 outline-none focus-visible:rounded-sm focus-visible:ring-2 focus-visible:ring-blue-500"
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
    numberDrafts[getDraftKey(procedure.id, field)] ?? (field === "minIntervalDays" ? procedure.minIntervalDays - 1 : procedure[field])

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
    onUpdateProcedureAction(procedureId, {[field]: field === "minIntervalDays" ? value + 1 : value})
  }

  return (
    <TooltipProvider delay={250}>
    <section aria-label="Каталог процедур" className="relative min-w-0 shrink-0" ref={trackRef}>
      <section className="sticky flex min-h-0 flex-col overflow-hidden rounded-[20px] bg-white" ref={panelRef} style={panelStyle}>
        <div className="min-h-0 min-w-0 flex-1 overflow-x-hidden">
          <div className="flex h-full min-h-0 w-full min-w-0 flex-col">
        {procedures.length > 0 && <div className={`z-10 hidden shrink-0 gap-2 border-b border-slate-200 px-2 py-3 font-medium text-slate-600 xl:grid ${catalogColumns}`}>
          <span>Название процедуры</span>
          <ProcedureColumnHelp field="duration" />
          <ProcedureColumnHelp field="preparation" />
          <ProcedureColumnHelp field="interval" />
          <ProcedureColumnHelp field="weekly" />
          <ProcedureColumnHelp field="sessions" />
          <ProcedureColumnHelp field="room" />
          <ProcedureColumnHelp field="relaxing" />
          <ProcedureColumnHelp field="prerequisite" />
          <span className="sr-only">Удалить</span>
        </div>}
        {procedures.length > 0 && (
          <div aria-label="Пояснения к параметрам процедур" className="grid shrink-0 grid-cols-1 gap-x-4 gap-y-1 border-b border-slate-200 px-3 py-2 sm:grid-cols-2 md:grid-cols-3 xl:hidden">
            <ProcedureColumnHelp field="duration" />
            <ProcedureColumnHelp field="preparation" />
            <ProcedureColumnHelp field="interval" />
            <ProcedureColumnHelp field="weekly" />
            <ProcedureColumnHelp field="sessions" />
            <ProcedureColumnHelp field="room" />
            <ProcedureColumnHelp field="relaxing" />
          <ProcedureColumnHelp field="prerequisite" />
          </div>
        )}
        <div className="min-h-0 flex-1 overflow-hidden" ref={viewportRef}>
          <div className="divide-y divide-slate-100" ref={contentRef}>
        {procedures.length ? procedures.map((procedure) => <article className={`grid min-w-0 gap-2 p-2 transition-colors focus-within:bg-blue-50/30 hover:bg-slate-50 sm:grid-cols-[minmax(0,1fr)_minmax(9rem,0.8fr)_minmax(9rem,0.8fr)] sm:items-center ${catalogColumns}`} key={procedure.id}>
          <Input aria-label="Название процедуры" className="h-10 min-w-0 rounded-lg px-3 text-sm" maxLength={480} onChange={(event) => onUpdateProcedureAction(procedure.id, {name: event.target.value})} value={procedure.name}/>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="shrink-0 xl:hidden">Время, мин</span>
            <Input aria-invalid={getDraftKey(procedure.id, "durationMinutes") in numberDrafts} aria-label={`Продолжительность процедуры ${procedure.name}, минут`} className="h-9 w-full min-w-0 flex-1 rounded-lg px-2 text-sm text-slate-900" max={420} min={1} onBlur={() => clearNumberDraft(procedure.id, "durationMinutes")} onChange={(event) => updateNumber(procedure.id, "durationMinutes", event.target.value)} step={1} title="От 1 до 420 минут, вместе с подготовкой не более 420" type="number" value={getNumberValue(procedure, "durationMinutes")}/>
          </Label>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="shrink-0 xl:hidden">Подгот., мин</span>
            <Input aria-invalid={getDraftKey(procedure.id, "preparationMinutes") in numberDrafts} aria-label={`Время подготовки к процедуре ${procedure.name}, минут`} className="h-9 w-full min-w-0 flex-1 rounded-lg px-2 text-sm text-slate-900" max={419} min={0} onBlur={() => clearNumberDraft(procedure.id, "preparationMinutes")} onChange={(event) => updateNumber(procedure.id, "preparationMinutes", event.target.value)} step={1} title={`Всего в расписании: ${procedure.durationMinutes + procedure.preparationMinutes} мин`} type="number" value={getNumberValue(procedure, "preparationMinutes")}/>
          </Label>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="shrink-0 xl:hidden">Перерыв, дн.</span>
            <Input aria-invalid={getDraftKey(procedure.id, "minIntervalDays") in numberDrafts} aria-label={`Дней перерыва между процедурами ${procedure.name}`} className="h-9 w-full min-w-0 flex-1 rounded-lg px-2 text-sm text-slate-900" max={364} min={0} onBlur={() => clearNumberDraft(procedure.id, "minIntervalDays")} onChange={(event) => updateNumber(procedure.id, "minIntervalDays", event.target.value)} step={1} title="0 — можно проводить каждый день" type="number" value={getNumberValue(procedure, "minIntervalDays")}/>
          </Label>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="shrink-0 xl:hidden">В неделю</span>
            <Input aria-invalid={getDraftKey(procedure.id, "defaultWeeklyCount") in numberDrafts} aria-label={`Количество процедур ${procedure.name} в неделю по умолчанию`} className="h-9 w-full min-w-0 flex-1 rounded-lg px-2 text-sm text-slate-900" max={7} min={1} onBlur={() => clearNumberDraft(procedure.id, "defaultWeeklyCount")} onChange={(event) => updateNumber(procedure.id, "defaultWeeklyCount", event.target.value)} step={1} title="От 1 до 7 процедур в неделю" type="number" value={getNumberValue(procedure, "defaultWeeklyCount")}/>
          </Label>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="shrink-0 xl:hidden">На курс</span>
            <Input aria-invalid={getDraftKey(procedure.id, "defaultSessionCount") in numberDrafts} aria-label={`Количество сеансов процедуры ${procedure.name} по умолчанию на курс`} className="h-9 w-full min-w-0 flex-1 rounded-lg px-2 text-sm text-slate-900" max={365} min={1} onBlur={() => clearNumberDraft(procedure.id, "defaultSessionCount")} onChange={(event) => updateNumber(procedure.id, "defaultSessionCount", event.target.value)} step={1} title="От 1 до 365 сеансов на курс; не более дней пребывания пациента" type="number" value={getNumberValue(procedure, "defaultSessionCount")}/>
          </Label>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="shrink-0 xl:hidden">Кабинет</span>
            <Input aria-label={`Кабинет процедуры ${procedure.name}`} className="h-9 w-full min-w-0 flex-1 rounded-lg px-2 text-sm text-slate-900" maxLength={80} onChange={(event) => onUpdateProcedureAction(procedure.id, {room: event.target.value})} placeholder="Любой" title="Необязательное поле: пустой кабинет — общий график организации" value={procedure.room ?? ""}/>
          </Label>
          <Label className="flex min-w-0 items-center gap-2 text-sm text-slate-600"><Switch aria-label={`Расслабляющая процедура: ${procedure.name}`} checked={procedure.isRelaxing} onCheckedChange={(checked) => onUpdateProcedureAction(procedure.id, {isRelaxing: checked})}/><span className="xl:hidden">Расслабляющая процедура</span></Label>
          <div className="min-w-0">
            <span className="mb-1 block text-xs text-slate-500 xl:hidden">Перед сеансом</span>
            <CatalogPrerequisiteSelect procedure={procedure} procedures={procedures}
              onChange={(id) => onUpdateProcedureAction(procedure.id, {defaultPrerequisiteId: id})}/>
          </div>
          <Button aria-label={`Удалить ${procedure.name}`} className="size-9 justify-self-end rounded-lg text-slate-400 hover:bg-red-50 hover:text-red-600 sm:justify-self-center" onClick={() => onRemoveProcedureAction(procedure.id)} size="icon" type="button" variant="ghost"><X className="size-4"/></Button>
        </article>) : <p className="p-6 text-center text-sm text-slate-500">Каталог пуст. Добавьте процедуру, чтобы продолжить.</p>}
          </div>
        </div>
          </div>
        </div>
      </section>
    </section>
    </TooltipProvider>
  )
}
