import { Tabs } from "@base-ui/react/tabs"
import {useRef, type PointerEvent as ReactPointerEvent} from "react"
import {
  ArrowLeft01StandardIcon as ChevronLeft,
  ArrowRight01StandardIcon as ChevronRight,
  Add01StrokeRoundedIcon as Plus,
} from "@/shared/lib/icons"
import { Button } from "@/shared/ui/shadcn/button"
import { formatWeekLabel } from "@/shared/lib/calendar-date"

type CalendarToolbarProps = {
  date: Date
  onMoveWeek: (offset: number) => void
  onToday: () => void
  onCreate: () => void
  onClearSchedule: () => void
  isClearingSchedule: boolean
  patientNumbers: number[]
  selectedPatientNumber: number | null
  onSelectPatient: (patientNumber: number | null) => void
}

const dateFormatter = new Intl.DateTimeFormat("ru-RU", {
  weekday: "long",
  day: "numeric",
  month: "long",
})

const patientTabClassName = "flex h-8 shrink-0 cursor-pointer items-center justify-center whitespace-nowrap rounded-lg px-3 text-sm font-medium text-slate-600 outline-none transition-colors hover:text-slate-900 focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-inset data-active:bg-white data-active:text-slate-950 data-active:shadow-sm"

export function CalendarToolbar({ date, onMoveWeek, onToday, onCreate, onClearSchedule, isClearingSchedule, patientNumbers, selectedPatientNumber, onSelectPatient }: CalendarToolbarProps) {
  const tabsScroller = useRef<HTMLDivElement>(null)
  const pointerDrag = useRef<{id: number; x: number; scrollLeft: number; moved: boolean; patient: number | null} | null>(null)
  const suppressTabClick = useRef(false)

  const startScroll = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.pointerType !== "mouse" || event.button !== 0) return
    pointerDrag.current = {id: event.pointerId, x: event.clientX, scrollLeft: event.currentTarget.scrollLeft, moved: false, patient: selectedPatientNumber}
  }
  const moveScroll = (event: ReactPointerEvent<HTMLDivElement>) => {
    const gesture = pointerDrag.current
    if (!gesture || gesture.id !== event.pointerId) return
    const delta = event.clientX - gesture.x
    if (!gesture.moved && Math.abs(delta) < 6) return
    if (!gesture.moved) {
      gesture.moved = true
      event.currentTarget.setPointerCapture(event.pointerId)
    }
    event.currentTarget.scrollLeft = gesture.scrollLeft - delta
  }
  const finishScroll = (event: ReactPointerEvent<HTMLDivElement>) => {
    const gesture = pointerDrag.current
    if (!gesture || gesture.id !== event.pointerId) return
    if (gesture.moved) {
      suppressTabClick.current = true
      // Base UI may activate a focused tab on pointer-down. A drag must
      // never change the active patient's calendar.
      onSelectPatient(gesture.patient)
      // Pointer capture can retarget the synthetic click to the scroller.
      // Keep suppression through the click event, then release it.
      window.setTimeout(() => {suppressTabClick.current = false}, 0)
    }
    pointerDrag.current = null
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
  }
  return (
    <div className="bg-white">
      <section aria-label="Управление календарём" className="flex flex-wrap items-center gap-x-4 gap-y-3 border-b border-slate-200 bg-white px-4 py-3 sm:px-6">
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
          <Tabs.Root
            className="order-3 min-w-0 w-full lg:order-none lg:w-auto lg:flex-1"
            value={selectedPatientNumber === null ? "all" : String(selectedPatientNumber)}
            onValueChange={(value) => {
              if (pointerDrag.current?.moved || suppressTabClick.current) return
              onSelectPatient(value === "all" ? null : Number(value))
            }}
          >
            <div
              ref={tabsScroller}
              className="max-w-full cursor-grab select-none overflow-x-auto rounded-xl bg-slate-100 p-1 active:cursor-grabbing [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
              onPointerDown={startScroll}
              onPointerMove={moveScroll}
              onPointerUp={finishScroll}
              onPointerCancel={finishScroll}
              onClickCapture={(event) => {
                if (!suppressTabClick.current) return
                event.preventDefault()
                event.stopPropagation()
                suppressTabClick.current = false
              }}
            >
              <Tabs.List aria-label="Фильтр расписания по пациенту" activateOnFocus className="flex min-w-max items-center gap-1">
                <Tabs.Tab
                  id="calendar-patient-tab-all"
                  aria-controls="calendar-appointments-panel"
                  value="all"
                  className={patientTabClassName}
                >
                  Все пациенты
                </Tabs.Tab>
                {patientNumbers.map((patientNumber) => (
                  <Tabs.Tab
                    key={patientNumber}
                    id={`calendar-patient-tab-${patientNumber}`}
                    aria-controls="calendar-appointments-panel"
                    value={String(patientNumber)}
                    className={patientTabClassName}
                  >
                    Пациент {patientNumber}
                  </Tabs.Tab>
                ))}
              </Tabs.List>
            </div>
          </Tabs.Root>
        )}
        <div className="ml-auto flex flex-wrap items-center gap-2 sm:flex-nowrap sm:justify-end">
          <Button className="h-10 rounded-xl px-3" disabled={isClearingSchedule} onClick={onClearSchedule} type="button" variant="destructive">
            Очистить расписание
          </Button>
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
