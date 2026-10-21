"use client"

import {useEffect} from "react"
import {Button} from "@/shared/ui/shadcn/button"
import {notify} from "@/shared/lib/notifications"

export default function PageError({reset}: {error: Error & {digest?: string}; reset: () => void}) {
  useEffect(() => {
    notify.error(
      "Не удалось открыть страницу",
      "Данные пока не загрузились. Попробуйте открыть страницу ещё раз.",
    )
  }, [])

  return (
    <main className="mx-auto flex min-h-[60svh] w-full max-w-xl flex-col items-start justify-center gap-4 px-6">
      <h1 className="text-xl font-semibold">Не удалось открыть страницу</h1>
      <p className="text-base leading-relaxed text-muted-foreground">
        Не получилось загрузить данные. Подождите немного и попробуйте снова.
      </p>
      <Button type="button" onClick={reset}>Попробовать снова</Button>
    </main>
  )
}
