import {formatUnscheduledDetails, type UnscheduledDetail} from "./schedule-feedback"

export type ApiFailure = {
  status?: number
  reason?: string
  path: string
  method: string
  unscheduledDetails?: UnscheduledDetail[]
  unscheduledCount?: number
}

export type UserErrorMessage = {
  title: string
  description: string
  type: "error" | "warning"
}

// Never show raw server errors, status codes, or internal component names to users.
export const explainApiError = ({status, reason = "", path, method, unscheduledDetails = [], unscheduledCount = 0}: ApiFailure): UserErrorMessage => {
  const message = reason.toLowerCase()
  const isProcedure = path.startsWith("/v1/procedures")
  const isPlan = path.startsWith("/v1/day-plans")
  const isSchedule = path.startsWith("/v1/schedules")
  const isAppointment = path.startsWith("/v1/appointments")
  const isSettings = path.startsWith("/v1/calendar-settings")
  const isAbsence = path.startsWith("/v1/patient-absences")

  if (message === "invalid response body") {
    return {type: "error", title: "Не удалось прочитать полученные данные", description: "Попробуйте повторить действие. Если проблема останется, обратитесь к ответственному сотруднику."}
  }
  if (status === undefined) {
    return {type: "error", title: "Не удалось связаться с программой", description: "Проверьте подключение и повторите действие."}
  }
  if (status === 409) {
    if (isProcedure && message.includes("catalog revision changed")) {
      return {type: "warning", title: "Каталог изменился в другом окне", description: "Сохранение остановлено. Обновите страницу и проверьте процедуры перед повторной правкой."}
    }
    const weeklyConflict = reason.match(/procedure "([^"]+)" exceeds (\d+) sessions in seven days/i)
    if (weeklyConflict) {
      const [, procedure, limit] = weeklyConflict
      return {type: "warning", title: "Нарушен недельный лимит сеансов",
        description: `«${procedure}»: допускается максимум ${limit} сеанс. за любые 7 последовательных дней. Перенесите запись на более позднюю дату или уменьшите число сеансов. Изменяйте предел в каталоге только после согласования с врачом.`}
    }
    const intervalConflict = reason.match(/procedure "([^"]+)" requires (\d+) full days between starts/i)
    if (intervalConflict) {
      const [, procedure, rawGap] = intervalConflict
      const gap = Number(rawGap)
      return {type: "warning", title: gap === 0 ? "Не более одного сеанса в день" : "Не соблюдён перерыв между сеансами",
        description: gap === 0
          ? `«${procedure}»: второй сеанс в тот же день запрещён, даже если в каталоге указан перерыв 0 дней. Выберите следующий разрешённый день.`
          : `«${procedure}»: между сеансами должно быть минимум ${gap} полных дней без этой процедуры. Перенесите повторный сеанс на более позднюю дату.`}
    }
    if (message.includes("requires a separate earlier") && message.includes("session on the same day")) {
      return {type: "warning", title: "Сначала нужна расслабляющая процедура", description: "Для этого пациента сложную процедуру можно провести только после отдельного расслабляющего сеанса в тот же день. Измените время связанной пары или перенесите обе процедуры."}
    }
    if ((isAppointment || isSchedule) && (message.includes("prerequisite") || message.includes("relaxing procedure"))) {
      return {type: "warning", title: "Нельзя нарушить порядок процедур", description: "Обязательная расслабляющая процедура должна предшествовать сложной в тот же день. Перенесите связанные сеансы вместе или выберите другое время."}
    }
    if (isAbsence && message.includes("manually edited")) {
      return {type: "warning", title: "Есть сеанс, изменённый вручную", description: "Он попадает в даты отсутствия. Сначала перенесите или отмените его в календаре, затем повторите перепланирование. Остальные записи не изменены."}
    }
    if (isAbsence) {
      return {type: "warning", title: "Расписание изменилось", description: "Обновите страницу и заново проверьте даты отсутствия. Уже внесённые изменения сохранены."}
    }
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
    if (isSchedule && (message.includes("manually protected") || message.includes("защищённые сеансы парой"))) {
      return {type: "warning", title: "Связанные сеансы изменены вручную", description: "Для сложной процедуры необходим более ранний расслабляющий сеанс в тот же день. Проверьте и при необходимости перенесите обе записи вместе, затем повторите расчёт."}
    }
    if (isPlan && message.includes("prerequisite must be a relaxing")) {
      return {type: "warning", title: "Неверно выбрана расслабляющая процедура", description: "Укажите в связке процедуру, отмеченную в каталоге как расслабляющая, либо отмените обязательную связку."}
    }
    if (isPlan && message.includes("not enough prerequisite sessions")) {
      return {type: "warning", title: "Недостаточно расслабляющих сеансов", description: "Для каждого сложного сеанса нужен один назначенный расслабляющий. Увеличьте количество расслабляющих сеансов или уменьшите количество сложных."}
    }
    if (isSchedule && message.includes("no procedures assigned to patients")) {
      return {type: "warning", title: "Нет назначенных процедур", description: "Добавьте пациенту процедуры и число сеансов, сохраните назначения и повторите составление расписания."}
    }
    if (isAbsence && message.includes("cannot be rescheduled")) {
      return {type: "warning", title: "Не удалось перенести все сеансы", description: "В оставшийся срок лечения не хватает свободного времени. Измените период отсутствия или срок лечения. Расписание не изменено."}
    }
    if (isAbsence) {
      return {type: "warning", title: "Даты отсутствия не подходят", description: "Выберите дни в пределах срока пребывания пациента. Уже прошедшие периоды нельзя отмечать задним числом."}
    }
    if (message.includes("outside organization hours") || message.includes("overlaps a closure")) {
      return {type: "warning", title: "Организация в это время не принимает", description: "Выберите рабочий день и время без перерыва. График работы можно изменить в настройках календаря."}
    }
    if (message.includes("outside patient treatment period")) {
      return {type: "warning", title: "Дата вне срока пребывания", description: "Выберите день, когда пациент ещё находится на лечении."}
    }
    if (isSchedule) {
      return {type: "warning", title: "Не все процедуры помещаются в курс", description: formatUnscheduledDetails(unscheduledDetails, unscheduledCount)}
    }
    return {type: "warning", title: "Эти данные пока нельзя сохранить", description: "Проверьте выбранные даты и количество процедур."}
  }
  if (status === 400) {
    if (isSchedule && message.includes("experimental solver supports at most 80 sessions")) {
      return {type: "warning", title: "Слишком большой курс для тестового оптимизатора", description: "Текущий локальный CP-SAT поддерживает до 80 сеансов за один расчёт. Уменьшите число назначений для проверки."}
    }
    if (isSchedule && message.includes("experimental solver supports at most 31 days")) {
      return {type: "warning", title: "Слишком длительный курс для тестового оптимизатора", description: "Текущая локальная версия CP-SAT рассчитывает курс продолжительностью до 31 дня."}
    }
    if (isAbsence) return {type: "warning", title: "Проверьте даты отсутствия", description: "Укажите даты начала и окончания и причину отсутствия."}
    if (isProcedure) return {type: "warning", title: "Проверьте данные процедуры", description: "Укажите название, длительность, интервал и количество сеансов в допустимых пределах."}
    if (isPlan && message.includes("treatment dates must fall")) return {type: "warning", title: "Даты вне периода плана", description: "Даты поступления и выезда должны быть в течение 365 дней после выбранной даты плана. Измените даты пациента или дату плана."}
    if (isPlan && message.includes("staydays does not match")) return {type: "warning", title: "Даты лечения не совпадают с длительностью", description: "Выберите срок пребывания от 1 до 365 дней и повторите сохранение."}
    if (isPlan && message.includes("invalid procedure prerequisite")) return {type: "warning", title: "Неверная зависимость процедур", description: "Назначьте пациенту обе процедуры: сложную и расслабляющую. Связка доступна для высокой тяжести."}
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
