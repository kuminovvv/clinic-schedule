CREATE INDEX IF NOT EXISTS day_plan_items_procedure_id_idx
    ON day_plan_items (procedure_id);

CREATE INDEX IF NOT EXISTS procedures_default_prerequisite_id_idx
    ON procedures (default_prerequisite_id)
    WHERE default_prerequisite_id IS NOT NULL;
