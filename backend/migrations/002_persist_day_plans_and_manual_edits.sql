ALTER TABLE appointments
    ADD COLUMN IF NOT EXISTS manually_modified BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS day_plans (
    schedule_date DATE PRIMARY KEY,
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    selected_patient_number INTEGER CHECK (selected_patient_number > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS day_plan_patients (
    schedule_date DATE NOT NULL REFERENCES day_plans(schedule_date) ON DELETE CASCADE,
    patient_number INTEGER NOT NULL CHECK (patient_number > 0),
    severity TEXT NOT NULL CHECK (severity IN ('high', 'medium', 'low')),
    PRIMARY KEY (schedule_date, patient_number)
);

CREATE TABLE IF NOT EXISTS day_plan_items (
    schedule_date DATE NOT NULL,
    patient_number INTEGER NOT NULL,
    procedure_id TEXT NOT NULL REFERENCES procedures(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (schedule_date, patient_number, procedure_id),
    FOREIGN KEY (schedule_date, patient_number)
        REFERENCES day_plan_patients(schedule_date, patient_number) ON DELETE CASCADE
);

