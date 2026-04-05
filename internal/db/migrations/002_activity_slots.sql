-- ── Activity Slots ─────────────────────────────────────────
CREATE TABLE IF NOT EXISTS activity_slots (
    slot_time   TEXT NOT NULL,
    operator    TEXT NOT NULL,
    cwd         TEXT,
    msg_count   INTEGER NOT NULL DEFAULT 0,
    first_text  TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_slot_dedup
    ON activity_slots(slot_time, operator);

CREATE INDEX IF NOT EXISTS idx_slot_day
    ON activity_slots(slot_time);

-- Clean up agent-created draft entries
DELETE FROM time_entries WHERE operator != 'human';

-- Update import_log dedup to track by source_path
DROP INDEX IF EXISTS idx_import_dedup;
CREATE UNIQUE INDEX IF NOT EXISTS idx_import_dedup
    ON import_log(source, source_path);
