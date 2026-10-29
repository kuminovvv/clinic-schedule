"use client"

import {useEffect, useState} from "react"
import {Button} from "@/shared/ui/shadcn/button"
import {Cancel01StrokeRoundedIcon} from "@/shared/lib/icons"

export const ScheduleWarningsPanel = ({courseVersion, validationWarnings}: {courseVersion: string; validationWarnings: string[]}) => {
  const [generationNotice, setGenerationNotice] = useState<{courseVersion: string; warnings: string[]}>({courseVersion, warnings: []})
  const [open, setOpen] = useState(false)
  const stale = generationNotice.warnings.length > 0 && generationNotice.courseVersion !== courseVersion
  const generationWarnings = generationNotice.courseVersion === courseVersion ? generationNotice.warnings : []
  const warnings = [...validationWarnings, ...generationWarnings]

  useEffect(() => {
    const handleWarnings = (event: Event) => {
      const detail = (event as CustomEvent<{warnings?: unknown; courseVersion?: string}>).detail
      const next = detail?.warnings
      const normalized = Array.isArray(next) ? next.filter((item): item is string => typeof item === "string" && item.trim().length > 0) : []
      setGenerationNotice({courseVersion: detail?.courseVersion ?? courseVersion, warnings: normalized})
      if (normalized.length) setOpen(true)
    }
    window.addEventListener("clinic:schedule-warnings", handleWarnings)
    return () => window.removeEventListener("clinic:schedule-warnings", handleWarnings)
  }, [courseVersion])

  if (!warnings.length && !open) return null

  return <>
    {warnings.length > 0 && <Button type="button" variant="outline" className="border-amber-300 bg-amber-50 text-amber-900 hover:bg-amber-100" onClick={() => setOpen(true)}>
      Проверить предупреждения <span className="ml-1 rounded-full bg-amber-200 px-2 py-0.5 text-xs tabular-nums">{warnings.length}</span>
    </Button>}
    {open && <div className="fixed inset-0 z-50" role="presentation">
      <button aria-label="Закрыть предупреждения" className="absolute inset-0 cursor-default bg-slate-950/20" onClick={() => setOpen(false)} type="button" />
      <aside aria-label="Предупреждения расписания" className="absolute inset-y-0 right-0 flex w-full max-w-md flex-col bg-white p-5 text-slate-900 shadow-2xl sm:p-6">
        <div className="flex items-start justify-between gap-4">
          <div><p className="text-xs font-semibold uppercase tracking-wider text-amber-700">Проверка курса</p><h2 className="mt-1 text-lg font-semibold">Предупреждения <span className="text-slate-500">{warnings.length}</span></h2><p className="mt-1 text-sm text-slate-500">Исправьте пункты и повторите проверку курса.</p></div>
          <Button aria-label="Закрыть панель предупреждений" className="size-8 rounded-lg" onClick={() => setOpen(false)} size="icon" type="button" variant="ghost"><Cancel01StrokeRoundedIcon className="size-4"/></Button>
        </div>
        <div className="mt-5 min-h-0 flex-1 space-y-3 overflow-y-auto pr-1">
          {stale && <p className="rounded-xl bg-slate-50 p-3 text-sm text-slate-600">Курс изменился. Запустите проверку ещё раз, чтобы обновить предупреждения.</p>}
          {!warnings.length && !stale && <p className="rounded-xl bg-slate-50 p-3 text-sm text-slate-600">Текущих предупреждений нет.</p>}
          {warnings.map((warning, index) => <div className="rounded-xl bg-amber-50 p-3 text-sm leading-relaxed text-amber-950" key={`${warning}-${index}`}>{warning}</div>)}
        </div>
      </aside>
    </div>}
  </>
}
