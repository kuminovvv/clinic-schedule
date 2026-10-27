ALTER TABLE day_plan_items
    ADD COLUMN prerequisite_procedure_id TEXT REFERENCES procedures(id) ON DELETE SET NULL;
