import type {Procedure} from "@/entities/procedure"

/** One session per day, at least minIntervalDays calendar days apart,
 * and no more than defaultWeeklyCount sessions in any rolling seven days. */
export const maximumSessionsInDays = (procedure: Procedure, days: number): number => {
  const starts: number[] = []
  for (let day = 0; day < Math.min(365, days); day++) {
    const last = starts.at(-1)
    if (last !== undefined && day - last < procedure.minIntervalDays) continue
    if (starts.filter((previous) => day - previous < 7).length >= procedure.defaultWeeklyCount) continue
    starts.push(day)
  }
  return starts.length
}

export const minimumDaysForSessions = (procedure: Procedure, count: number): number | null => {
  if (!Number.isInteger(count) || count < 1 || count > 365) return null
  const starts: number[] = []
  for (let day = 0; day < 365; day++) {
    const last = starts.at(-1)
    if (last !== undefined && day - last < procedure.minIntervalDays) continue
    if (starts.filter((previous) => day - previous < 7).length >= procedure.defaultWeeklyCount) continue
    starts.push(day)
    if (starts.length === count) return day + 1
  }
  return null
}

export const frequencyRuleLabel = (procedure: Procedure): string => {
  const gap = procedure.minIntervalDays - 1
  const repetition = gap === 0
    ? "не более одного сеанса в день"
    : `не менее ${gap} полных дней перерыва между сеансами`
  return `${repetition}; не более ${procedure.defaultWeeklyCount} за любые 7 дней`
}

export const frequencyProblem = (
  procedure: Procedure, requested: number, stayDays: number,
): {maximum: number; minimumDays: number | null} | null => {
  const maximum = maximumSessionsInDays(procedure, stayDays)
  return requested > maximum ? {maximum, minimumDays: minimumDaysForSessions(procedure, requested)} : null
}

export const frequencyAdvice = (
  procedure: Procedure, requested: number, stayDays: number,
): string | null => {
  const issue = frequencyProblem(procedure, requested, stayDays)
  if (!issue) return null
  const recommended = issue.minimumDays === null
    ? "Такое количество не поместится даже в пределах 365 дней."
    : `Для ${requested} сеансов нужно минимум ${issue.minimumDays} дней без учёта выходных и занятости.`
  return `Правила каталога: ${frequencyRuleLabel(procedure)}. За ${stayDays} дней можно провести максимум ${issue.maximum} сеансов. ${recommended}`
}
