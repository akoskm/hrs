CREATE TABLE IF NOT EXISTS time_off_types (
    id         TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    is_system  INTEGER NOT NULL DEFAULT 0,
    archived_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TRIGGER IF NOT EXISTS trg_time_off_types_updated
    AFTER UPDATE ON time_off_types FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE time_off_types SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = NEW.id;
END;

CREATE UNIQUE INDEX IF NOT EXISTS idx_time_off_types_project_name
    ON time_off_types(project_id, name) WHERE archived_at IS NULL;

CREATE TABLE IF NOT EXISTS time_off_days (
    id               TEXT PRIMARY KEY NOT NULL,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    time_off_type_id TEXT NOT NULL REFERENCES time_off_types(id) ON DELETE CASCADE,
    day              TEXT NOT NULL,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TRIGGER IF NOT EXISTS trg_time_off_days_updated
    AFTER UPDATE ON time_off_days FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE time_off_days SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = NEW.id;
END;

CREATE UNIQUE INDEX IF NOT EXISTS idx_time_off_days_project_day
    ON time_off_days(project_id, day);

CREATE INDEX IF NOT EXISTS idx_time_off_days_day
    ON time_off_days(day);
