import {Suspense} from "react"
import {SettingsRouteContent} from "@/features/manage-calendar-settings/ui/settings-route-content"

export default function SettingsPage() {
  return <Suspense fallback={null}><SettingsRouteContent/></Suspense>
}
