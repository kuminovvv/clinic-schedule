import {Suspense} from "react"
import {PatientsRouteContent} from "@/features/schedule-patients/ui/patients-route-content"

export default function SchedulePatientsRoutePage({searchParams}: PageProps<"/schedule/patients">) {
  return <Suspense fallback={<main className="flex min-h-svh items-center justify-center bg-background text-sm text-slate-500">Загружаем пациентов…</main>}><PatientsRouteContent searchParams={searchParams}/></Suspense>
}
