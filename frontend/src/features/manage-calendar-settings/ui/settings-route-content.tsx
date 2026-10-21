import {CalendarSettingsPage} from "./calendar-settings-page"
import {getServerData} from "@/shared/api/backend"
import type {TimeStepMinutes} from "@/entities/calendar-settings"

export const SettingsRouteContent = async () => {
  const settings = await getServerData<{timeStepMinutes: TimeStepMinutes; displayStartHour: number; displayEndHour: number}>("/v1/calendar-settings")
  return <CalendarSettingsPage initialSettings={settings}/>
}
