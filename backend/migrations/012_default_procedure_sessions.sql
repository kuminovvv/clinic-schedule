ALTER TABLE procedures
    ADD COLUMN IF NOT EXISTS default_session_count INTEGER NOT NULL DEFAULT 1
    CHECK (default_session_count BETWEEN 1 AND 365);
