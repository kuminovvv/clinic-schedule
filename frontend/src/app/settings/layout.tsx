import type { ReactNode } from "react"
import { SettingsSidebarLayout } from "@/features/manage-calendar-settings/ui/settings-sidebar"

export default function SettingsLayout({ children }: { children: ReactNode }) {
  return <SettingsSidebarLayout>{children}</SettingsSidebarLayout>
}
