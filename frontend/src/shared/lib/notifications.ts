import {toast} from "@/shared/ui/shadcn/toast"

type Notice = {
  title: string
  description: string
}

type NoticeType = "error" | "warning" | "success" | "info" | "constraint"

const recentNotices = new Map<string, number>()

const showNotice = (type: NoticeType, notice: Notice) => {
  const key = `${type}:${notice.title}:${notice.description}`
  const now = Date.now()
  if (now - (recentNotices.get(key) ?? 0) < 3500) return
  recentNotices.set(key, now)
  if (recentNotices.size > 30) {
    for (const [previousKey, timestamp] of recentNotices) {
      if (now - timestamp > 3500) recentNotices.delete(previousKey)
    }
  }
  toast.add({type, title: notice.title, description: notice.description, timeout: type === "constraint" ? 15000 : undefined})
}

export const notify = {
  error: (title: string, description: string) => showNotice("error", {title, description}),
  warning: (title: string, description: string) => showNotice("warning", {title, description}),
  success: (title: string, description: string) => showNotice("success", {title, description}),
  info: (title: string, description: string) => showNotice("info", {title, description}),
  constraint: (title: string, description: string) => showNotice("constraint", {title, description}),
  confirmConstraint: (id: string, title: string, description: string, onConfirm: () => void) => {
    toast.add({
      id,
      type: "constraint",
      title,
      description,
      priority: "high",
      timeout: 0,
      actionProps: {
        children: "Продолжить",
        onClick: () => {
          toast.close(id)
          onConfirm()
        },
      },
    })
  },
  dismiss: (id: string) => toast.close(id),
}
