-- Keep schedule_date as the stable key for a treatment course. Each patient
-- now owns an inclusive arrival/departure interval inside that course.
ALTER TABLE day_plan_patients
    ADD COLUMN IF NOT EXISTS arrival_date DATE,
    ADD COLUMN IF NOT EXISTS discharge_date DATE;

UPDATE day_plan_patients
SET arrival_date = schedule_date,
    discharge_date = schedule_date + stay_days - 1
WHERE arrival_date IS NULL OR discharge_date IS NULL;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM day_plan_patients
        WHERE stay_days NOT BETWEEN 1 AND 365
           OR discharge_date < arrival_date
           OR discharge_date - arrival_date NOT BETWEEN 0 AND 364
    ) THEN
        RAISE EXCEPTION 'existing patient treatment dates are outside the supported 1..365 day range';
    END IF;

    IF EXISTS (
        SELECT 1 FROM patient_absences a
        JOIN day_plan_patients p USING (schedule_date, patient_number)
        WHERE a.from_date < p.arrival_date OR a.to_date > p.discharge_date
    ) THEN
        RAISE EXCEPTION 'existing patient absences are outside the migrated treatment range';
    END IF;
END $$;

ALTER TABLE day_plan_patients
    ADD CONSTRAINT day_plan_patients_treatment_dates_check
    CHECK (
        (arrival_date IS NULL AND discharge_date IS NULL)
        OR (
            arrival_date IS NOT NULL
            AND discharge_date IS NOT NULL
            AND discharge_date >= arrival_date
            AND discharge_date - arrival_date BETWEEN 0 AND 364
        )
    );
