"use client"

import Link from "next/link"
import {usePathname} from "next/navigation"
import {
  Calendar04StrokeRoundedIcon as Calendar04Icon,
  MagicWand01StrokeRoundedIcon as WandSparkles,
  Settings01StrokeRoundedIcon as Settings2,
} from "@/shared/lib/icons"
import {Button} from "@/shared/ui/shadcn/button"

const navigation = [
    {href: "/", label: "Календарь", Icon: Calendar04Icon},
    {href: "/schedule/patients", activePath: "/schedule", label: "Составить курс", Icon: WandSparkles},
    {href: "/settings", label: "Настройки", Icon: Settings2},
]

export function AppHeader() {
    const pathname = usePathname()

    return (
        <header className="fixed inset-x-0 top-0 z-40 h-16 bg-background">
            <div className="container mx-auto flex h-full items-center justify-between gap-3  md:px-0 p-4">
                <Link aria-label="Clinic Schedule — календарь" className="flex min-w-0 items-center gap-3" href="/">
          <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-blue-600 text-white">
            <Calendar04Icon className="size-5"/>
          </span>
                    <span className="min-w-0">
            <span
                className="block truncate text-base font-semibold tracking-tight text-slate-900">Clinic Schedule</span>
            <span className="hidden text-xs text-slate-500 sm:block">Расписание лечебницы</span>
          </span>
                </Link>
                <nav aria-label="Основная навигация" className="flex shrink-0 items-center gap-1 sm:gap-2">
                    {navigation.map(({href, activePath, label, Icon}) => {
                        const isActive = href === "/" ? pathname === "/" : pathname.startsWith(activePath ?? href)
                        return (
                            <Button
                                aria-label={label}
                                aria-current={isActive ? "page" : undefined}
                                key={href}
                                nativeButton={false}
                                render={<Link href={href}/>}
                                variant={isActive ? "white" : "ghost"}
                            >
                                <Icon className="size-4 sm:hidden"/>
                                <span className="hidden sm:inline">{label}</span>
                            </Button>
                        )
                    })}
                </nav>
            </div>
        </header>
    )
}
