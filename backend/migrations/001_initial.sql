CREATE TABLE IF NOT EXISTS procedures (
    id TEXT PRIMARY KEY,
    name VARCHAR(480) NOT NULL CHECK (length(trim(name)) > 0),
    duration_minutes INTEGER NOT NULL CHECK (duration_minutes BETWEEN 1 AND 420),
    is_relaxing BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS appointments (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    start_at TIMESTAMP WITHOUT TIME ZONE NOT NULL,
    end_at TIMESTAMP WITHOUT TIME ZONE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL CHECK (color IN ('blue', 'teal', 'orange', 'purple')),
    status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'cancelled')),
    generated_by_planner BOOLEAN NOT NULL DEFAULT FALSE,
    manually_modified BOOLEAN NOT NULL DEFAULT FALSE,
    patient_number INTEGER CHECK (patient_number > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_at > start_at)
);

CREATE INDEX IF NOT EXISTS appointments_start_at_idx ON appointments (start_at);
CREATE INDEX IF NOT EXISTS appointments_generated_date_idx ON appointments (start_at, generated_by_planner);

CREATE TABLE IF NOT EXISTS schedule_reports (
    schedule_date DATE PRIMARY KEY,
    scheduled_count INTEGER NOT NULL DEFAULT 0 CHECK (scheduled_count >= 0),
    unscheduled_count INTEGER NOT NULL DEFAULT 0 CHECK (unscheduled_count >= 0),
    unscheduled_patient_numbers INTEGER[] NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

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
    FOREIGN KEY (schedule_date, patient_number) REFERENCES day_plan_patients(schedule_date, patient_number) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS calendar_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    time_step_minutes INTEGER NOT NULL DEFAULT 10 CHECK (time_step_minutes IN (5, 10, 15, 30)),
    display_start_hour INTEGER NOT NULL DEFAULT 8 CHECK (display_start_hour BETWEEN 0 AND 23),
    display_end_hour INTEGER NOT NULL DEFAULT 18 CHECK (display_end_hour BETWEEN 1 AND 24),
    CHECK (display_end_hour > display_start_hour)
);

INSERT INTO calendar_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
