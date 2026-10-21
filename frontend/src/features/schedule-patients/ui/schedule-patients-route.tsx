import { SchedulePatientsPage } from "./schedule-patients-page"
import { getServerData } from "@/shared/api/backend"
import type { Procedure } from "@/entities/procedure"
import type {SavedDayPlan} from "../model/types"

export const SchedulePatientsRoute = async ({ tab, dateKey }: { tab: "patients" | "procedures"; dateKey: string }) => {
  const [procedures, initialPlan] = await Promise.all([
    getServerData<Procedure[]>("/v1/procedures"),
    getServerData<SavedDayPlan>("/v1/day-plans?date=" + dateKey),
  ])
  return <SchedulePatientsPage key={dateKey} dateKey={dateKey} initialPlan={initialPlan} procedures={procedures} tab={tab} />
}
