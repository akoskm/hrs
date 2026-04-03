-- hrs schema v1
-- Migration: 001_initial_schema.sql

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

-- ── Clients ─────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS clients (
    id          TEXT PRIMARY KEY NOT NULL,
    name        TEXT NOT NULL UNIQUE,
    contact     TEXT,
    archived_at TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TRIGGER IF NOT EXISTS trg_clients_updated
    AFTER UPDATE ON clients FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE clients SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = NEW.id;
END;

-- ── Projects ────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS projects (
    id               TEXT PRIMARY KEY NOT NULL,
    client_id        TEXT REFERENCES clients(id) ON DELETE SET NULL,
    name             TEXT NOT NULL,
    code             TEXT UNIQUE,
    hourly_rate      INTEGER NOT NULL DEFAULT 0,
    currency         TEXT NOT NULL DEFAULT 'EUR',
    billable_default INTEGER NOT NULL DEFAULT 1,
    color            TEXT,
    archived_at      TEXT,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TRIGGER IF NOT EXISTS trg_projects_updated
    AFTER UPDATE ON projects FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE projects SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = NEW.id;
END;

-- ── Tasks ───────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    code        TEXT,
    done        INTEGER NOT NULL DEFAULT 0,
    archived_at TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TRIGGER IF NOT EXISTS trg_tasks_updated
    AFTER UPDATE ON tasks FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE tasks SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = NEW.id;
END;

CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_project_code
    ON tasks(project_id, code) WHERE code IS NOT NULL;

-- ── Time Entries ────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS time_entries (
    id            TEXT PRIMARY KEY NOT NULL,
    project_id    TEXT REFERENCES projects(id) ON DELETE SET NULL,  -- nullable: drafts
    task_id       TEXT REFERENCES tasks(id) ON DELETE SET NULL,
    description   TEXT,
    started_at    TEXT NOT NULL,
    ended_at      TEXT,
    duration_secs INTEGER,
    billable      INTEGER NOT NULL DEFAULT 1,
    status        TEXT NOT NULL DEFAULT 'draft',     -- draft | confirmed
    operator      TEXT NOT NULL DEFAULT 'human',     -- human | claude-code | codex | ...
    source_ref    TEXT,                              -- external session ID for dedup
    worktree      TEXT,
    git_branch    TEXT,
    cwd           TEXT,
    metadata      TEXT,                              -- JSON blob
    deleted_at    TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TRIGGER IF NOT EXISTS trg_entries_updated
    AFTER UPDATE ON time_entries FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE time_entries SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = NEW.id;
END;

CREATE INDEX IF NOT EXISTS idx_entries_started
    ON time_entries(started_at) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_entries_project
    ON time_entries(project_id, started_at) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_entries_billable
    ON time_entries(billable, started_at) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_entries_status
    ON time_entries(status) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_entries_operator
    ON time_entries(operator, started_at) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_entries_source
    ON time_entries(operator, source_ref) WHERE source_ref IS NOT NULL AND deleted_at IS NULL;

-- ── Labels ──────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS labels (
    id         TEXT PRIMARY KEY NOT NULL,
    name       TEXT NOT NULL UNIQUE,
    color      TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- ── Time Entry <-> Label ────────────────────────────────────

CREATE TABLE IF NOT EXISTS time_entry_labels (
    time_entry_id TEXT NOT NULL REFERENCES time_entries(id) ON DELETE CASCADE,
    label_id      TEXT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (time_entry_id, label_id)
);

CREATE INDEX IF NOT EXISTS idx_tel_entry ON time_entry_labels(time_entry_id);
CREATE INDEX IF NOT EXISTS idx_tel_label ON time_entry_labels(label_id);

-- ── Import Log ──────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS import_log (
    id              TEXT PRIMARY KEY NOT NULL,
    source          TEXT NOT NULL,
    source_path     TEXT,
    session_id      TEXT NOT NULL,
    entries_created INTEGER NOT NULL DEFAULT 0,
    imported_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_import_dedup
    ON import_log(source, session_id);

-- ── Project Paths ───────────────────────────────────────────

CREATE TABLE IF NOT EXISTS project_paths (
    id         TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    path       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- ── Schema Version ──────────────────────────────────────────

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY NOT NULL,
    applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT OR IGNORE INTO schema_migrations (version) VALUES (1);
