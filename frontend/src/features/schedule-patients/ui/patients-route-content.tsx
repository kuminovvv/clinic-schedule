import {connection} from "next/server"
import {SchedulePatientsRoute} from "./schedule-patients-route"
import {getClinicDateKey, isDateKey} from "@/shared/api/backend"

export const PatientsRouteContent = async ({searchParams}: {searchParams: PageProps<"/schedule/patients">["searchParams"]}) => {
  const params = await searchParams
  const dateKey = isDateKey(params.date) ? params.date : undefined
  if (!dateKey) await connection()
  return <SchedulePatientsRoute dateKey={dateKey ?? getClinicDateKey()} tab="patients"/>
}
