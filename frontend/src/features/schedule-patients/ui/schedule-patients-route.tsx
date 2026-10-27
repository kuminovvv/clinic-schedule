import { SchedulePatientsPage } from "./schedule-patients-page"
import { getServerData } from "@/shared/api/backend"
import type { Procedure } from "@/entities/procedure"
import type {SavedDayPlan} from "../model/types"

export const SchedulePatientsRoute = async ({ tab, dateKey, selectedPatientNumber }: { tab: "patients" | "procedures"; dateKey: string; selectedPatientNumber?: number }) => {
  const [procedures, initialPlan] = await Promise.all([
    getServerData<Procedure[]>("/v1/procedures"),
    getServerData<SavedDayPlan>("/v1/day-plans?date=" + dateKey),
  ])
  const plan = selectedPatientNumber !== undefined && initialPlan.plans.some((item) => item.patientNumber === selectedPatientNumber)
    ? {...initialPlan, selectedPatientNumber}
    : initialPlan
  return <SchedulePatientsPage key={`${dateKey}:${tab}:${selectedPatientNumber ?? ""}`} dateKey={dateKey} initialPlan={plan} procedures={procedures} tab={tab} />
}
