import {connection} from "next/server"
import {CalendarPage} from "./calendar-page"
import {getClinicDateKey, isDateKey} from "@/shared/api/backend"

export const CalendarRouteContent = async ({searchParams}: {searchParams: PageProps<"/">["searchParams"]}) => {
  const params = await searchParams
  const dateKey = isDateKey(params.date) ? params.date : undefined
  if (!dateKey) await connection()
  return <CalendarPage dateKey={dateKey ?? getClinicDateKey()}/>
}
