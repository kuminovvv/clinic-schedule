import {connection} from "next/server"
import {SchedulePatientsRoute} from "./schedule-patients-route"
import {getClinicDateKey, isDateKey} from "@/shared/api/backend"

export const PatientsRouteContent = async ({searchParams}: {searchParams: PageProps<"/schedule/patients">["searchParams"]}) => {
  const params = await searchParams
  const dateKey = isDateKey(params.date) ? params.date : undefined
  const patientNumber = typeof params.patient === "string" && /^[1-9]\d*$/.test(params.patient)
    ? Number(params.patient) : undefined
  if (!dateKey) await connection()
  return <SchedulePatientsRoute dateKey={dateKey ?? getClinicDateKey()} selectedPatientNumber={patientNumber} tab="patients"/>
}
