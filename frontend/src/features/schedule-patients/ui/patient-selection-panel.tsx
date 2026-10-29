"use client"

import {Fragment} from "react"
import type {PatientPlan, Severity} from "../model/types"
import type {Procedure} from "@/entities/procedure"
import {Add01StrokeRoundedIcon as Plus, Cancel01StrokeRoundedIcon as X} from "@/shared/lib/icons"
import {Button} from "@/shared/ui/shadcn/button"
import {Label} from "@/shared/ui/shadcn/label"
import {Input} from "@/shared/ui/shadcn/input"
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from "@/shared/ui/shadcn/select"
import {Switch} from "@/shared/ui/shadcn/switch"
import {PatientTreatmentRange} from "./patient-treatment-range"
import {PatientAbsenceControl} from "./patient-absence-control"
import {formatDateLabel, inclusiveDayCount} from "@/shared/lib/date-range"
import {requiredRelaxingSessions} from "../model/patient-plan-rules"

const severityLabels: Record<Severity, string> = {high: "Высокая", medium: "Средняя", low: "Низкая"}

type PatientSelectionPanelProps = {
	courseKey: string
  plans: PatientPlan[]
  procedures: Procedure[]
  selectedPatientNumber: number | null
  onSelectPatientAction: (patientNumber: number | null) => void
  onAddPatientAction: () => void
  onRemovePatientAction: (patientNumber: number) => void
  onChangeSeverityAction: (patientNumber: number, severity: Severity) => void
	onChangeTreatmentRangeAction: (patientNumber: number, arrivalDate: string, departureDate: string) => void
  onChangeProcedureCountAction: (patientNumber: number, procedureId: string, count: number) => void
  onChangeProcedurePrerequisiteAction: (patientNumber: number, procedureId: string, prerequisiteId: string | null) => void
  onToggleProcedureAction: (procedureId: string, checked: boolean) => void
}

export const PatientSelectionPanel = ({courseKey, plans, procedures, selectedPatientNumber, onSelectPatientAction, onAddPatientAction, onRemovePatientAction, onChangeSeverityAction, onChangeTreatmentRangeAction, onChangeProcedureCountAction, onChangeProcedurePrerequisiteAction, onToggleProcedureAction}: PatientSelectionPanelProps) => {
  const selectedPlan = plans.find((plan) => plan.patientNumber === selectedPatientNumber)
  const treatmentDays = selectedPlan ? inclusiveDayCount(selectedPlan.arrivalDate, selectedPlan.departureDate) : 0
  const datesReady = treatmentDays >= 1 && treatmentDays <= 365
  const requiredRelaxing = selectedPlan ? requiredRelaxingSessions(selectedPlan) : {}
  const relaxingOptions = procedures.filter((procedure) => procedure.isRelaxing)
  return (
    <div className="grid gap-4 lg:min-h-0 lg:flex-1 lg:grid-cols-[minmax(18rem,22rem)_minmax(0,1fr)] lg:grid-rows-1">
      <aside aria-label="Список пациентов" className="flex min-h-0 flex-col rounded-[20px] bg-white p-3">
        <div className="mb-3 flex items-center justify-between gap-2">
          <div><h2 className="text-sm font-semibold">Пациенты</h2><p className="text-xs text-slate-500">{plans.length} в списке</p></div>
          <Button className="h-8 rounded-lg px-2.5 text-xs" onClick={onAddPatientAction} type="button" variant="outline"><Plus/>Добавить</Button>
        </div>
        <div className="max-h-[35vh] space-y-2 overflow-y-auto pr-1 lg:min-h-0 lg:max-h-none lg:flex-1">
          {plans.map((plan) => {
            const isSelected = plan.patientNumber === selectedPatientNumber
            return (
              <div className={`rounded-lg border p-2 ${isSelected ? "border-blue-200 bg-blue-50/60" : "border-slate-200 bg-white"}`} key={plan.patientNumber}>
                <div className="flex min-w-0 items-center gap-1">
                  <Button aria-pressed={isSelected} className="h-auto min-w-0 flex-1 justify-start rounded-md px-2 py-1.5 text-left hover:bg-white/80" onClick={() => onSelectPatientAction(plan.patientNumber)} type="button" variant="ghost">
                    <span className="min-w-0"><span className="block whitespace-nowrap text-sm font-medium">Пациент {plan.patientNumber}</span><span className="block truncate text-xs font-normal text-slate-500">Поступление: {formatDateLabel(plan.arrivalDate)} · выезд: {formatDateLabel(plan.departureDate)} · {plan.procedureIds.reduce((sum, id) => sum + (plan.procedureCounts[id] ?? 1), 0)} сеансов</span></span>
                  </Button>
                  <Select onValueChange={(value) => value && onChangeSeverityAction(plan.patientNumber, value as Severity)} value={plan.severity}>
                    <SelectTrigger aria-label={`Степень тяжести: пациент ${plan.patientNumber}`} className="h-8 w-25 shrink-0 rounded-md px-2 text-xs"><SelectValue>{severityLabels[plan.severity]}</SelectValue></SelectTrigger>
                    <SelectContent>{(Object.keys(severityLabels) as Severity[]).map((level) => <SelectItem key={level} value={level}>{severityLabels[level]}</SelectItem>)}</SelectContent>
                  </Select>
                  <Button aria-label={`Удалить пациента ${plan.patientNumber}`} className="size-8 shrink-0 rounded-md text-slate-400 hover:bg-red-50 hover:text-red-600" onClick={() => onRemovePatientAction(plan.patientNumber)} size="icon" type="button" variant="ghost"><X className="size-4"/></Button>
                </div>
              </div>
            )
          })}
          {!plans.length && <p className="rounded-lg border border-dashed border-slate-300 px-3 py-6 text-center text-sm text-slate-500">Добавьте пациента, чтобы назначить процедуры.</p>}
        </div>
      </aside>
      <section aria-label="Назначение процедур" className="flex min-h-0 min-w-0 flex-col rounded-[20px] bg-white p-3 sm:p-4">
        {selectedPlan ? <Fragment key={selectedPlan.patientNumber}>
          <div className="mb-3 flex flex-wrap items-start justify-between gap-2"><h2 className="text-sm font-semibold">Процедуры пациента {selectedPlan.patientNumber}</h2><span className="rounded-full bg-slate-100 px-2.5 py-1 text-xs text-slate-600">Выбрано: {selectedPlan.procedureIds.length} видов</span></div>
	          <PatientTreatmentRange
	            courseKey={courseKey}
	            arrivalDate={selectedPlan.arrivalDate}
	            departureDate={selectedPlan.departureDate}
	            onChange={(arrivalDate, departureDate) => onChangeTreatmentRangeAction(selectedPlan.patientNumber, arrivalDate, departureDate)}
	          />
          {datesReady ? <>
	          <PatientAbsenceControl courseKey={courseKey} patientNumber={selectedPlan.patientNumber} arrivalDate={selectedPlan.arrivalDate} departureDate={selectedPlan.departureDate}/>
          <div className="max-h-[40vh] overflow-y-auto lg:min-h-0 lg:max-h-none lg:flex-1">
            {procedures.length ? <div className="grid gap-2 xl:grid-cols-2">{procedures.map((procedure) => {
              const selected = selectedPlan.procedureIds.includes(procedure.id)
              const count = selectedPlan.procedureCounts[procedure.id] ?? 1
              const automaticallyAssigned = selected && procedure.isRelaxing && (requiredRelaxing[procedure.id] ?? 0) > 0
              return <div className="min-w-0 rounded-lg border border-slate-200 px-2 py-2 text-sm text-slate-700 hover:bg-slate-50" key={procedure.id}>
                <div className="flex min-w-0 items-center gap-2">
                  <Switch aria-label={`Процедура: ${procedure.name}`} checked={selected} onCheckedChange={(checked) => onToggleProcedureAction(procedure.id, checked)} size="sm"/>
                  <div className="min-w-0 flex-1"><span className="block truncate font-medium" title={procedure.name}>{procedure.name}</span></div>
                  {selected && <Label className="flex shrink-0 items-center gap-1 text-xs" title={automaticallyAssigned ? `Для связанных процедур требуется минимум ${requiredRelaxing[procedure.id]} сеансов.` : undefined}>Сеансов
                    <Input aria-label={`Количество сеансов: ${procedure.name}`} className="h-8 w-16 rounded-md px-2 text-sm" type="number" step={1} min={automaticallyAssigned ? requiredRelaxing[procedure.id] : 1} max={365} value={count} onChange={(event) => onChangeProcedureCountAction(selectedPlan.patientNumber, procedure.id, Number(event.target.value))}/>
                  </Label>}
                </div>
                {automaticallyAssigned && <p className="mt-1 pl-7 text-xs text-slate-500">Для подготовки нужно не меньше {requiredRelaxing[procedure.id]} сеансов.</p>}
                {selected && selectedPlan.severity === "high" && !procedure.isRelaxing && (relaxingOptions.length > 0 || selectedPlan.procedurePrerequisites?.[procedure.id]) && <div className="mt-2 pl-7">
                  <Label className="mb-1 block text-xs text-slate-600">Подготовка перед процедурой</Label>
                  <Select onValueChange={(value) => value && onChangeProcedurePrerequisiteAction(selectedPlan.patientNumber, procedure.id, value === "catalog" ? "catalog" : value === "none" ? null : value)}
                    value={Object.prototype.hasOwnProperty.call(selectedPlan.procedurePrerequisiteOverrides ?? {}, procedure.id)
                      ? selectedPlan.procedurePrerequisiteOverrides?.[procedure.id] ?? "none" : "catalog"}>
                    <SelectTrigger aria-label={`Расслабляющая процедура перед ${procedure.name}`} className="h-8 w-full rounded-md px-2 text-xs sm:max-w-72">
                      <SelectValue>{(() => {
                        const override = selectedPlan.procedurePrerequisiteOverrides?.[procedure.id]
                        const hasOverride = Object.prototype.hasOwnProperty.call(selectedPlan.procedurePrerequisiteOverrides ?? {}, procedure.id)
                        const assignedName = relaxingOptions.find((option) => option.id === override)?.name
                        const catalogName = relaxingOptions.find((option) => option.id === procedure.defaultPrerequisiteId)?.name
                        return !hasOverride ? `По каталогу: ${catalogName ?? "не требуется"}` : override === null ? "Не требуется" : assignedName ?? "Процедура недоступна"
                      })()}</SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="catalog">По каталогу: {relaxingOptions.find((option) => option.id === procedure.defaultPrerequisiteId)?.name ?? "не требуется"}</SelectItem>
                      <SelectItem value="none">Не требуется (для этого пациента)</SelectItem>
                      {relaxingOptions.map((relaxing) =>
                        <SelectItem key={relaxing.id} value={relaxing.id}>{relaxing.name}</SelectItem>
                      )}
                    </SelectContent>
                  </Select>
                </div>}
              </div>
            })}</div> : <p className="px-3 py-8 text-center text-sm text-slate-500">В каталоге пока нет процедур. Добавьте их в разделе «Каталог процедур».</p>}
          </div>
          </> : <div className="flex min-h-40 items-center justify-center rounded-xl bg-slate-50 px-5 text-center text-sm text-slate-600">Сначала выберите даты поступления и выезда. После этого станут доступны процедуры пациента.</div>}
        </Fragment> : <div className="flex min-h-48 items-center justify-center text-center text-sm text-slate-500">Выберите пациента или добавьте нового.</div>}
      </section>
    </div>
  )
}
