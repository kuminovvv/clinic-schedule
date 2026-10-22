"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import {
  ArrowLeft01StandardIcon as ArrowLeft,
  Calendar04StrokeRoundedIcon as Calendar04Icon,
} from "@/shared/lib/icons"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
} from "@/shared/ui/shadcn/sidebar"

export function SettingsSidebarLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()

  return (
    <div className="container settings-layout-container mx-auto min-h-[calc(100svh-4rem)]">
      <SidebarProvider className="min-h-[calc(100svh-4rem)] w-full">
      <Sidebar className="!left-[var(--settings-sidebar-offset)] top-16 bottom-0 h-auto" collapsible="icon" variant="floating">
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Настройки</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={pathname === "/settings"}
                    render={<Link href="/settings" />}
                  >
                    <Calendar04Icon />
                    <span>Календарь</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton render={<Link href="/" />}>
                <ArrowLeft />
                <span>Вернуться в календарь</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </Sidebar>
      <SidebarInset className="min-h-[calc(100svh-4rem)] bg-transparent">
        <div className="m-3 flex min-h-[calc(100svh-5.5rem)] flex-1 flex-col overflow-clip rounded-[24px] bg-white">
          <div className="flex h-12 shrink-0 items-center gap-2 border-b border-slate-100 bg-white px-4 sm:px-6">
            <SidebarTrigger aria-label="Переключить меню настроек" />
            <span className="text-sm font-medium">Настройки</span>
          </div>
          {children}
        </div>
      </SidebarInset>
      </SidebarProvider>
    </div>
  )
}
