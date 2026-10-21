ALTER TABLE day_plan_items
    DROP CONSTRAINT IF EXISTS day_plan_items_procedure_id_fkey;

ALTER TABLE day_plan_items
    ADD CONSTRAINT day_plan_items_procedure_id_fkey
    FOREIGN KEY (procedure_id) REFERENCES procedures(id) ON DELETE RESTRICT;
