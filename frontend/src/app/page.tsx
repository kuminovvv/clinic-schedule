import {Suspense} from "react"
import {CalendarRouteContent} from "@/widgets/calendar/ui/calendar-route-content"

export default function Page({searchParams}: PageProps<"/">) {
  return <Suspense fallback={null}><CalendarRouteContent searchParams={searchParams}/></Suspense>
}
