import {Suspense} from "react"
import {PatientsRouteContent} from "@/features/schedule-patients/ui/patients-route-content"

export default function SchedulePatientsRoutePage({searchParams}: PageProps<"/schedule/patients">) {
  return <Suspense fallback={null}><PatientsRouteContent searchParams={searchParams}/></Suspense>
}
