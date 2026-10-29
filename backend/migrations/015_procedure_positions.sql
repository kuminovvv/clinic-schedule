ALTER TABLE procedures
    ADD COLUMN IF NOT EXISTS position INTEGER;

WITH ordered AS (
    SELECT id, row_number() OVER (ORDER BY id) - 1 AS position
    FROM procedures
)
UPDATE procedures AS p
SET position = ordered.position
FROM ordered
WHERE p.id = ordered.id
  AND p.position IS NULL;

ALTER TABLE procedures
    ALTER COLUMN position SET DEFAULT 0,
    ALTER COLUMN position SET NOT NULL,
    ADD CONSTRAINT procedures_position_nonnegative CHECK (position >= 0);

CREATE INDEX IF NOT EXISTS procedures_position_idx ON procedures (position, id);
