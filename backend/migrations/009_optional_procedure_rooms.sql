-- Rooms are optional. Empty string means unrestricted organization-wide pool.
-- Appointments keep a snapshot of the room so later catalog edits do not
-- silently move previously booked sessions to another room.
ALTER TABLE procedures
    ADD COLUMN IF NOT EXISTS room TEXT NOT NULL DEFAULT '' CHECK (length(room) <= 80);

ALTER TABLE appointments
    ADD COLUMN IF NOT EXISTS room TEXT NOT NULL DEFAULT '' CHECK (length(room) <= 80);

CREATE INDEX IF NOT EXISTS appointments_room_start_idx
    ON appointments (lower(btrim(room)), start_at)
    WHERE status = 'scheduled' AND btrim(room) <> '';
