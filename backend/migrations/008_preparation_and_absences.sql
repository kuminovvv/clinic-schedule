-- Preserve the previously reserved time of every existing procedure.
-- A legacy 50-minute procedure becomes 30 minutes of treatment + 20 of preparation.
ALTER TABLE procedures ADD COLUMN IF NOT EXISTS preparation_minutes INTEGER;
UPDATE procedures
SET preparation_minutes = CASE WHEN duration_minutes > 20 THEN 20 ELSE 0 END,
    duration_minutes = CASE WHEN duration_minutes > 20 THEN duration_minutes - 20 ELSE duration_minutes END
WHERE preparation_minutes IS NULL;
ALTER TABLE procedures ALTER COLUMN preparation_minutes SET DEFAULT 20;
ALTER TABLE procedures ALTER COLUMN preparation_minutes SET NOT NULL;
ALTER TABLE procedures ADD CONSTRAINT procedures_preparation_range
    CHECK (preparation_minutes BETWEEN 0 AND 419 AND duration_minutes + preparation_minutes <= 420);

CREATE TABLE patient_absences (
    schedule_date DATE NOT NULL REFERENCES day_plans(schedule_date) ON DELETE CASCADE,
    patient_number INTEGER NOT NULL CHECK (patient_number > 0),
    from_date DATE NOT NULL,
    to_date DATE NOT NULL,
    reason TEXT NOT NULL DEFAULT 'illness' CHECK (reason IN ('illness','other')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_date, patient_number, from_date, to_date),
    FOREIGN KEY (schedule_date, patient_number)
        REFERENCES day_plan_patients(schedule_date, patient_number) ON DELETE CASCADE,
    CHECK (to_date >= from_date)
);
CREATE INDEX patient_absences_lookup ON patient_absences(schedule_date,patient_number,from_date,to_date);
