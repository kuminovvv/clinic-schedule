ALTER TABLE appointments
    ADD COLUMN IF NOT EXISTS source_plan_date DATE;

CREATE INDEX IF NOT EXISTS appointments_source_plan_date_idx
    ON appointments (source_plan_date) WHERE generated_by_planner AND NOT manually_modified;
