"use client"

import type {Procedure} from "@/entities/procedure"
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from "@/shared/ui/shadcn/select"

type Props = {
  procedure: Procedure
  procedures: Procedure[]
  onChange: (id: string) => void
}

export const CatalogPrerequisiteSelect = ({procedure, procedures, onChange}: Props) => {
  const relaxing = procedures.filter((candidate) => candidate.isRelaxing && candidate.id !== procedure.id)
  const selected = relaxing.find((candidate) => candidate.id === procedure.defaultPrerequisiteId)
  if (procedure.isRelaxing) return <span className="text-sm text-slate-400">—</span>

  return (
    <Select value={procedure.defaultPrerequisiteId || "none"} onValueChange={(value) => {
      if (value) onChange(value === "none" ? "" : value)
    }}>
      <SelectTrigger
        aria-label={`Подготовительная расслабляющая для ${procedure.name}`}
        className="h-9 w-full min-w-0 max-w-full overflow-hidden rounded-lg px-2 text-xs"
      >
        <SelectValue className="min-w-0 max-w-full truncate">{selected?.name ?? "Не требуется"}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="none">Не требуется</SelectItem>
        {relaxing.map((candidate) =>
          <SelectItem key={candidate.id} value={candidate.id}>{candidate.name}</SelectItem>
        )}
      </SelectContent>
    </Select>
  )
}
