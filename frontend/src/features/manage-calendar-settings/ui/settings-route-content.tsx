import {CalendarSettingsPage} from "./calendar-settings-page"
import {getServerData} from "@/shared/api/backend"
import type {CalendarSettings} from "@/entities/calendar-settings"

export const SettingsRouteContent = async () => {
  const settings = await getServerData<CalendarSettings>("/v1/calendar-settings")
  return <CalendarSettingsPage initialSettings={settings}/>
}
