import {Suspense} from "react"
import {ProceduresRouteContent} from "@/features/schedule-patients/ui/procedures-route-content"

export default function ScheduleProceduresRoutePage({searchParams}: PageProps<"/schedule/procedures">) {
  return <Suspense fallback={<main className="flex min-h-svh items-center justify-center bg-background text-sm text-slate-500">Загружаем каталог процедур…</main>}><ProceduresRouteContent searchParams={searchParams}/></Suspense>
}
