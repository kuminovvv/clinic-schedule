CREATE TABLE IF NOT EXISTS procedure_catalog_revision (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0)
);

INSERT INTO procedure_catalog_revision (id, revision) VALUES (1, 0)
ON CONFLICT (id) DO NOTHING;
