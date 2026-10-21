import {
  ArrowLeft01StrokeRoundedIcon as ChevronLeft,
  ArrowRight01StrokeRoundedIcon as ChevronRight,
  Add01StrokeRoundedIcon as Plus,
} from "@/shared/lib/icons"
import { Button } from "@/shared/ui/shadcn/button"
import { formatWeekLabel } from "@/shared/lib/calendar-date"

type CalendarToolbarProps = {
  date: Date
  onMoveWeek: (offset: number) => void
  onToday: () => void
  onCreate: () => void
  patientNumbers: number[]
  selectedPatientNumber: number | null
  onSelectPatient: (patientNumber: number | null) => void
}

const dateFormatter = new Intl.DateTimeFormat("ru-RU", {
  weekday: "long",
  day: "numeric",
  month: "long",
})

export function CalendarToolbar({ date, onMoveWeek, onToday, onCreate, patientNumbers, selectedPatientNumber, onSelectPatient }: CalendarToolbarProps) {
  return (
    <div className="bg-white">
      <section aria-label="Управление календарём" className="flex flex-col gap-3 border-b border-slate-200 bg-white px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-3">
          <nav aria-label="Навигация по календарю" className="flex shrink-0 items-center gap-2 sm:gap-3">
            <Button className="h-10 shrink-0 rounded-xl px-4" onClick={onToday} variant="outline">
              Сегодня
            </Button>
            <div className="flex h-10 shrink-0 items-center rounded-xl border border-slate-200 p-0">
              <Button aria-label="Предыдущая неделя" className="size-9 rounded-lg" onClick={() => onMoveWeek(-1)} variant="ghost">
                <ChevronLeft />
              </Button>
              <Button aria-label="Следующая неделя" className="size-9 rounded-lg" onClick={() => onMoveWeek(1)} variant="ghost">
                <ChevronRight />
              </Button>
            </div>
            <div className="min-w-28 text-center sm:min-w-36 sm:px-1">
              <p className="text-sm font-semibold capitalize text-slate-900">{formatWeekLabel(date)}</p>
              <p className="truncate text-xs capitalize text-slate-500">{dateFormatter.format(date)}</p>
            </div>
          </nav>
          {patientNumbers.length > 0 && (
            <nav aria-label="Фильтр расписания по пациенту" className="flex min-w-0 max-w-full gap-2 overflow-x-auto py-0.5 sm:flex-1">
              <Button aria-pressed={selectedPatientNumber === null} className="h-8 shrink-0 rounded-lg px-3 text-xs" onClick={() => onSelectPatient(null)} type="button" variant={selectedPatientNumber === null ? "secondary" : "outline"}>Все пациенты</Button>
              {patientNumbers.map((patientNumber) => (
                <Button aria-pressed={selectedPatientNumber === patientNumber} className="h-8 shrink-0 rounded-lg px-3 text-xs" key={patientNumber} onClick={() => onSelectPatient(patientNumber)} type="button" variant={selectedPatientNumber === patientNumber ? "secondary" : "outline"}>Пациент {patientNumber}</Button>
              ))}
            </nav>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2 sm:flex-nowrap sm:justify-end">
          <Button className="h-10 rounded-xl bg-blue-600 px-4 text-white hover:bg-blue-700" onClick={onCreate}>
            <Plus />
            <span className="hidden sm:inline">Новая запись</span>
            <span className="sm:hidden">Создать</span>
          </Button>
        </div>
      </section>
    </div>
  )
}
