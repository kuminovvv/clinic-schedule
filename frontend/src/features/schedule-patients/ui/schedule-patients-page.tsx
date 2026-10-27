"use client"

import Link from "next/link"
import {useRouter} from "next/navigation"
import type {MouseEvent} from "react"
import {Add01StrokeRoundedIcon as Plus} from "@/shared/lib/icons"
import type {Procedure} from "@/entities/procedure"
import {Button} from "@/shared/ui/shadcn/button"
import {DatePicker} from "@/shared/ui/shadcn/date-picker"
import {Label} from "@/shared/ui/shadcn/label"
import type {SavedDayPlan} from "../model/types"
import {PatientSelectionPanel} from "./patient-selection-panel"
import {ProcedureCatalog} from "./procedure-catalog"
import {useSchedulePatients} from "./use-schedule-patients"

type Props = {
    procedures: Procedure[]
    initialPlan: SavedDayPlan
    dateKey: string
    tab: "patients" | "procedures"
}

export const SchedulePatientsPage = ({procedures, initialPlan, dateKey, tab}: Props) => {
    const router = useRouter()
    const {plans, procedureDrafts, selectedPatientNumber, setSelectedPatientNumber, planConflict, isGenerating, addPatient, removePatient, changeSeverity, changeTreatmentRange, changeProcedureCount, changeProcedurePrerequisite, toggleProcedure, updateProcedure, addProcedure, removeProcedure, validProcedures, flushProcedureSaves, flushPlanSaves, changeCourseDate, submit} = useSchedulePatients({procedures, initialPlan, dateKey})
    const handleTabNavigation = async (event: MouseEvent<HTMLAnchorElement>, href: string) => {
        if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
        event.preventDefault()
        if (await flushProcedureSaves() && await flushPlanSaves()) router.push(href)
    }
    const view = tab
    return (
        <main
            className={`flex min-h-[calc(100svh-4rem)] flex-col bg-background px-4 py-6 text-slate-900 sm:p-4 ${view === "patients" ? "h-[calc(100svh-4rem)] overflow-hidden" : ""}`}>
            <section className="container mx-auto flex min-h-0 w-full flex-1 flex-col">

                <div
                    className="mb-4 flex shrink-0 flex-col gap-3 rounded-[20px] bg-white p-2 sm:flex-row sm:items-center sm:justify-between">
                    <nav aria-label="Раздел расписания" className="grid w-full grid-cols-2 gap-1 rounded-[16px] bg-slate-100 p-1 sm:inline-flex sm:w-auto">
                        {([
                            {key: "patients", label: "Пациенты", href: `/schedule/patients?date=${dateKey}`},
                            {key: "procedures", label: "Каталог процедур", href: `/schedule/procedures?date=${dateKey}`},
                        ] as const).map(({key, label, href}) => (
                            <Link
                                key={key}
                                href={href}
                                prefetch={false}
                                onClick={(event) => void handleTabNavigation(event, href)}
                                aria-current={view === key ? "page" : undefined}
                                className={`flex h-10 min-w-0 items-center justify-center rounded-xl px-3 text-center text-[13px] font-medium transition-colors duration-150 outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 sm:px-5 sm:text-sm ${
                                    view === key
                                        ? "bg-blue-600 text-white shadow-sm"
                                        : "text-slate-600 hover:bg-white hover:text-slate-900 active:bg-slate-200"
                                }`}
                            >
                                {label}
                            </Link>
                        ))}
                    </nav>
                    <div className="flex items-center justify-end gap-2 sm:ml-auto">
                        {view === "patients" && (
                            <Label className="flex items-center gap-2 text-xs text-slate-600">
                                Дата плана
                                <DatePicker ariaLabel="Дата плана" className="h-9 w-36 rounded-lg px-2 text-sm" dateKey={dateKey} disabled={isGenerating || planConflict} onChange={(nextDate) => void changeCourseDate(nextDate)}/>
                            </Label>
                        )}
                        {view === "patients" && (
                            <Button className="rounded-xl bg-blue-600 px-4 text-white hover:bg-blue-700"
                                disabled={!plans.some((plan) => plan.procedureIds.length) || !validProcedures || planConflict || isGenerating}
                                    onClick={submit} type="button">
                                {isGenerating ? "Составляем…" : "Составить курс процедур"}
                            </Button>
                        )}
                        {view === "procedures" && (
                            <Button className="rounded-xl" onClick={addProcedure} type="button" variant="outline">
                                <Plus/>
                                Добавить процедуру
                            </Button>
                        )}
                    </div>
                </div>

                {view === "patients" ? (
                    <PatientSelectionPanel
                        courseKey={dateKey}
                        onAddPatientAction={addPatient}
                        onChangeSeverityAction={changeSeverity}
                        onChangeTreatmentRangeAction={changeTreatmentRange}
                        onChangeProcedureCountAction={changeProcedureCount}
                        onChangeProcedurePrerequisiteAction={changeProcedurePrerequisite}
                        onRemovePatientAction={removePatient}
                        onSelectPatientAction={setSelectedPatientNumber}
                        onToggleProcedureAction={toggleProcedure}
                        plans={plans}
                        procedures={procedureDrafts}
                        selectedPatientNumber={selectedPatientNumber}
                    />
                ) : (
                    <ProcedureCatalog
                        onRemoveProcedureAction={removeProcedure}
                        onUpdateProcedureAction={updateProcedure}
                        procedures={procedureDrafts}
                    />
                )}
            </section>
        </main>
    )
};
