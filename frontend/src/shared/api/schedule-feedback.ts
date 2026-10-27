export type UnscheduledDetail = {
  patientNumber: number
  procedureName: string
  requestedCount: number
  missingCount: number
  reason?: string
  recommendation?: string
}

export const formatUnscheduledDetails = (details: UnscheduledDetail[], unscheduledCount: number): string => {
  const lines = details.slice(0, 8).map((item) => [
    `Пациент ${item.patientNumber}: «${item.procedureName}» — не размещено ${item.missingCount} из ${item.requestedCount} сеансов.`,
    item.reason ? `Причина: ${item.reason}` : "",
    item.recommendation ? `Рекомендация: ${item.recommendation}` : "",
  ].filter(Boolean).join("\n"))
  if (details.length > 8) lines.push(`Дополнительно не размещены процедуры ещё в ${details.length - 8} строках отчёта.`)
  if (!lines.length) lines.push(`Не удалось разместить ${unscheduledCount} сеансов.`)
  if (!details.some((detail) => detail.recommendation)) {
    lines.push("Уменьшите число сеансов, увеличьте срок лечения или скорректируйте интервалы и доступность расписания.")
  }
  return lines.join("\n")
}
