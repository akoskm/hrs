-- ── Activity Slot Details ───────────────────────────────────
ALTER TABLE activity_slots ADD COLUMN git_branch TEXT;
ALTER TABLE activity_slots ADD COLUMN user_texts TEXT;
ALTER TABLE activity_slots ADD COLUMN token_input INTEGER DEFAULT 0;
ALTER TABLE activity_slots ADD COLUMN token_output INTEGER DEFAULT 0;
