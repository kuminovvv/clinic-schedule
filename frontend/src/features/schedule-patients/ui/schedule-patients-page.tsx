"use client"

import Link from "next/link"
import {Add01StrokeRoundedIcon as Plus} from "@/shared/lib/icons"
import type {Procedure} from "@/entities/procedure"
import {Button} from "@/shared/ui/shadcn/button"
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
    const {plans, procedureDrafts, selectedPatientNumber, setSelectedPatientNumber, saveError, planConflict, isGenerating, addPatient, removePatient, changeSeverity, changeStay, changeProcedureCount, toggleProcedure, updateProcedure, addProcedure, removeProcedure, validProcedures, validPlans, submit} = useSchedulePatients({procedures, initialPlan, dateKey})
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
                            <Button className="rounded-xl bg-blue-600 px-4 text-white hover:bg-blue-700"
                                disabled={!plans.some((plan) => plan.procedureIds.length) || !validProcedures || !validPlans || planConflict || isGenerating}
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
                        admissionDateKey={dateKey}
                        onAddPatientAction={addPatient}
                        onChangeSeverityAction={changeSeverity}
                        onChangeStayAction={changeStay}
                        onChangeProcedureCountAction={changeProcedureCount}
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
                {view === "procedures" && !validProcedures && <p className="text-sm text-red-600" role="alert">Проверьте длительность (1–420 минут), интервал (1–365 дней) и число процедур в неделю (1–7).</p>}
                {planConflict ? (
                    <div className="mt-3 flex flex-wrap items-center gap-3 text-sm text-red-600" role="alert">
                        <span>Данные изменились в другом окне. Чтобы избежать потери данных, сохранение остановлено.</span>
                        <Button onClick={() => window.location.reload()} type="button" variant="outline">Обновить данные</Button>
                    </div>
                ) : saveError ? (
                    <p className="mt-3 text-sm text-red-600" role="alert">Некоторые изменения не сохранились. Проверьте уведомление и попробуйте изменить данные ещё раз.</p>
                ) : (
                    <p className="sr-only" role="status">Изменения сохраняются автоматически.</p>
                )}
            </section>
        </main>
    )
};
