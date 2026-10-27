import {DayPicker, type DayPickerProps} from "react-day-picker"
import {cn} from "@/shared/lib/utils"

type CalendarProps = DayPickerProps

function Calendar({className, ...props}: CalendarProps) {
  return (
    <DayPicker
      className={cn("[--rdp-accent-color:#2563eb] [--rdp-accent-background-color:#dbeafe]", className)}
      {...props}
    />
  )
}

export {Calendar}
