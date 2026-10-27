-- A patient can override the catalog requirement, including explicitly
-- turning it off (override_set=true with override_id=NULL).
ALTER TABLE day_plan_items
    ADD COLUMN IF NOT EXISTS prerequisite_override_set BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS prerequisite_override_id TEXT REFERENCES procedures(id) ON DELETE RESTRICT;

ALTER TABLE day_plan_items
    ADD CONSTRAINT day_plan_items_override_value_check
    CHECK (prerequisite_override_set OR prerequisite_override_id IS NULL);

-- Existing linked patient treatments were explicitly configured before the
-- catalog supported defaults. Keep them as patient-specific overrides.
UPDATE day_plan_items
SET prerequisite_override_set = TRUE,
    prerequisite_override_id = prerequisite_procedure_id
WHERE prerequisite_procedure_id IS NOT NULL;
