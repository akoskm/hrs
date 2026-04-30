CREATE TABLE IF NOT EXISTS time_off_allowances (
    id               TEXT PRIMARY KEY NOT NULL,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    time_off_type_id TEXT NOT NULL REFERENCES time_off_types(id) ON DELETE CASCADE,
    year             INTEGER NOT NULL CHECK (year > 0),
    allowed_days     INTEGER NOT NULL CHECK (allowed_days >= 0),
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TRIGGER IF NOT EXISTS trg_time_off_allowances_updated
    AFTER UPDATE ON time_off_allowances FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE time_off_allowances SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = NEW.id;
END;

CREATE UNIQUE INDEX IF NOT EXISTS idx_time_off_allowances_project_type_year
    ON time_off_allowances(project_id, time_off_type_id, year);
