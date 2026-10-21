"use client"

import {Toast as ToastPrimitive} from "@base-ui/react/toast"
import {Cancel01StrokeRoundedIcon as CloseIcon} from "@/shared/lib/icons"
import {cn} from "@/shared/lib/utils"

// Based on the official shadcn/ui Base UI Toast component.
export const toast = ToastPrimitive.createToastManager()

const ToastList = () => {
  const {toasts} = ToastPrimitive.useToastManager()

  return toasts.map((item) => (
    <ToastPrimitive.Root
      key={item.id}
      toast={item}
      data-slot="toast"
      className={cn(
        "group/toast pointer-events-auto absolute right-0 bottom-0 z-[calc(1000-var(--toast-index))] w-full origin-bottom overflow-visible rounded-xl border border-border bg-popover text-popover-foreground shadow-lg outline-none",
        item.type === "constraint" && "border-amber-300 bg-amber-50 text-amber-950",
        "[--gap:0.75rem] [--height:var(--toast-frontmost-height,var(--toast-height))] [--offset-y:calc(var(--toast-offset-y)*-1+calc(var(--toast-index)*var(--gap)*-1)+var(--toast-swipe-movement-y))] [--peek:0.75rem] [--scale:calc(max(0,1-(var(--toast-index)*0.1)))] [--shrink:calc(1-var(--scale))]",
        "h-(--height) [transform:translateX(var(--toast-swipe-movement-x))_translateY(calc(var(--toast-swipe-movement-y)-(var(--toast-index)*var(--peek))-(var(--shrink)*var(--height))))_scale(var(--scale))]",
        "[transition:transform_500ms_cubic-bezier(0.22,1,0.36,1),opacity_500ms,height_150ms] after:absolute after:top-full after:left-0 after:h-[calc(var(--gap)+1px)] after:w-full after:content-['']",
        "data-expanded:h-(--toast-height) data-expanded:[transform:translateX(var(--toast-swipe-movement-x))_translateY(var(--offset-y))]",
        "data-limited:opacity-0 data-starting-style:[transform:translateY(150%)]",
        "[&[data-ending-style]:not([data-limited]):not([data-swipe-direction])]:[transform:translateY(150%)]",
        "data-ending-style:data-[swipe-direction=down]:[transform:translateY(calc(var(--toast-swipe-movement-y)+150%))]",
        "data-ending-style:data-[swipe-direction=left]:[transform:translateX(calc(var(--toast-swipe-movement-x)-150%))_translateY(var(--offset-y))]",
        "data-ending-style:data-[swipe-direction=right]:[transform:translateX(calc(var(--toast-swipe-movement-x)+150%))_translateY(var(--offset-y))]",
        "data-ending-style:data-[swipe-direction=up]:[transform:translateY(calc(var(--toast-swipe-movement-y)-150%))]",
      )}
    >
      <ToastPrimitive.Content className="flex h-full items-start gap-3 overflow-hidden p-4 transition-opacity data-behind:opacity-0 data-expanded:opacity-100">
        <span
          aria-hidden="true"
          className={cn(
            "mt-1.5 size-2 shrink-0 rounded-full bg-blue-600",
            item.type === "error" && "bg-red-600",
            item.type === "warning" && "bg-amber-600",
            item.type === "constraint" && "bg-amber-600",
            item.type === "success" && "bg-emerald-600",
          )}
        />
        <div className="min-w-0 flex-1 space-y-1">
          <ToastPrimitive.Title className="text-base font-semibold leading-6"/>
          <ToastPrimitive.Description className={cn("max-h-56 overflow-y-auto whitespace-pre-line text-[15px] leading-6 text-muted-foreground", item.type === "constraint" && "text-amber-950/85")}/>
          {item.actionProps && (
            <ToastPrimitive.Action
              className="mt-3 min-h-10 rounded-lg bg-amber-900 px-4 text-sm font-medium text-white hover:bg-amber-950 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-amber-800"
            />
          )}
        </div>
        <ToastPrimitive.Close
          aria-label="Закрыть уведомление"
          className="flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2"
        >
          <CloseIcon aria-hidden="true" className="size-4"/>
        </ToastPrimitive.Close>
      </ToastPrimitive.Content>
    </ToastPrimitive.Root>
  ))
}

export const Toaster = () => (
  <ToastPrimitive.Provider toastManager={toast} timeout={10000}>
    <ToastPrimitive.Portal>
      <ToastPrimitive.Viewport
        data-slot="toast-viewport"
        className="pointer-events-none fixed inset-x-4 bottom-4 z-50 mx-auto w-auto max-w-md outline-none sm:right-5 sm:left-auto sm:mx-0 sm:w-full"
      >
        <ToastList/>
      </ToastPrimitive.Viewport>
    </ToastPrimitive.Portal>
  </ToastPrimitive.Provider>
)
