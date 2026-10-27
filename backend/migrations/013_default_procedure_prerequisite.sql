-- The catalog can prescribe one relaxing treatment before another treatment.
-- NULL in storage represents no default prerequisite; the API exposes "".
ALTER TABLE procedures
    ADD COLUMN IF NOT EXISTS default_prerequisite_id TEXT
        REFERENCES procedures(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE procedures
    ADD CONSTRAINT procedures_default_prerequisite_not_self
        CHECK (default_prerequisite_id IS NULL OR default_prerequisite_id <> id);
