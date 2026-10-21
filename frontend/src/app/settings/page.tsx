import {Suspense} from "react"
import {SettingsRouteContent} from "@/features/manage-calendar-settings/ui/settings-route-content"

export default function SettingsPage() {
  return <Suspense fallback={<main className="flex min-h-svh items-center justify-center bg-background text-sm text-slate-500">Загружаем настройки…</main>}><SettingsRouteContent/></Suspense>
}
