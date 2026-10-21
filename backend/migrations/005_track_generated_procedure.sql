ALTER TABLE appointments
    ADD COLUMN IF NOT EXISTS source_procedure_id TEXT;
