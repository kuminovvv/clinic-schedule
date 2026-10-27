import {Suspense} from "react"
import {ProceduresRouteContent} from "@/features/schedule-patients/ui/procedures-route-content"

export default function ScheduleProceduresRoutePage({searchParams}: PageProps<"/schedule/procedures">) {
  return <Suspense fallback={null}><ProceduresRouteContent searchParams={searchParams}/></Suspense>
}
