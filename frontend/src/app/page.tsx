import {Suspense} from "react"
import {CalendarRouteContent} from "@/widgets/calendar/ui/calendar-route-content"

export default function Page({searchParams}: PageProps<"/">) {
  return <Suspense fallback={<main className="flex min-h-svh items-center justify-center bg-background text-sm text-slate-500">Загружаем расписание…</main>}><CalendarRouteContent searchParams={searchParams}/></Suspense>
}
