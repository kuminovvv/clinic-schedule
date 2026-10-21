export type ApiFailure = {
  status?: number
  reason?: string
  path: string
  method: string
}

export type UserErrorMessage = {
  title: string
  description: string
  type: "error" | "warning"
}

// Never show raw server errors, status codes, or internal component names to users.
export const explainApiError = ({status, reason = "", path, method}: ApiFailure): UserErrorMessage => {
  const message = reason.toLowerCase()
  const isProcedure = path.startsWith("/v1/procedures")
  const isPlan = path.startsWith("/v1/day-plans")
  const isSchedule = path.startsWith("/v1/schedules")
  const isAppointment = path.startsWith("/v1/appointments")
  const isSettings = path.startsWith("/v1/calendar-settings")

  if (message === "invalid response body") {
    return {type: "error", title: "Не удалось прочитать полученные данные", description: "Попробуйте повторить действие. Если проблема останется, обратитесь к ответственному сотруднику."}
  }
  if (status === undefined) {
    return {type: "error", title: "Не удалось связаться с программой", description: "Проверьте подключение и повторите действие."}
  }
  if (status === 409) {
    if (isProcedure && method === "DELETE") {
      return {type: "warning", title: "Процедуру пока нельзя удалить", description: "Она уже назначена пациенту. Сначала уберите её из назначений, затем попробуйте снова."}
    }
    if (isPlan || message.includes("changed concurrently")) {
      return {type: "warning", title: "Данные изменились в другом окне", description: "Чтобы не потерять чужие изменения, сохранение остановлено. Обновите страницу и проверьте данные."}
    }
    if (message.includes("minimum interval") || message.includes("cannot repeat")) {
      return {type: "warning", title: "Эту процедуру пока нельзя повторить", description: "Между сеансами должен пройти установленный интервал. Выберите более поздний день."}
    }
    if (message.includes("removed from catalog")) {
      return {type: "warning", title: "Процедура больше не доступна", description: "Её удалили из каталога. Обновите страницу и выберите другую процедуру."}
    }
    if (message.includes("occupied") || message.includes("conflict")) {
      return {type: "warning", title: "Выбранное время занято", description: "Выберите другое время или день и повторите попытку."}
    }
    return {type: "warning", title: "Не удалось выполнить изменение", description: "Данные могли измениться. Обновите страницу и попробуйте ещё раз."}
  }
  if (status === 422) {
    if (message.includes("outside patient treatment period")) {
      return {type: "warning", title: "Дата вне срока пребывания", description: "Выберите день, когда пациент ещё находится на лечении."}
    }
    if (isSchedule) {
      return {type: "warning", title: "Не все процедуры помещаются в курс", description: "Уменьшите количество сеансов, увеличьте срок пребывания или измените интервалы."}
    }
    return {type: "warning", title: "Эти данные пока нельзя сохранить", description: "Проверьте выбранные даты и количество процедур."}
  }
  if (status === 400) {
    if (isProcedure) return {type: "warning", title: "Проверьте данные процедуры", description: "Укажите название, длительность, интервал и количество сеансов в допустимых пределах."}
    if (isPlan || isSchedule) return {type: "warning", title: "Проверьте назначения пациентов", description: "Проверьте срок пребывания, количество сеансов и выбранные процедуры."}
    if (isSettings) return {type: "warning", title: "Проверьте настройки календаря", description: "Выберите допустимое время начала и окончания рабочего дня."}
    return {type: "warning", title: "Проверьте введённые данные", description: "Исправьте заполненные поля и попробуйте ещё раз."}
  }
  if (status === 401 || status === 403) {
    return {type: "warning", title: "Недостаточно прав", description: "Вы не можете выполнить это действие. Обратитесь к ответственному сотруднику."}
  }
  if (status === 404) {
    return {type: "warning", title: "Запись не найдена", description: "Возможно, её уже удалили. Обновите страницу и проверьте список."}
  }
  if (status === 429) {
    return {type: "warning", title: "Слишком много действий подряд", description: "Подождите немного и попробуйте снова."}
  }
  if (status >= 500) {
    return {type: "error", title: "Программа временно недоступна", description: "Изменение не удалось сохранить. Подождите немного и повторите попытку."}
  }
  return {
    type: "error",
    title: isAppointment ? "Не удалось сохранить запись" : "Не удалось выполнить действие",
    description: "Попробуйте ещё раз. Если проблема повторится, обратитесь к ответственному сотруднику.",
  }
}
