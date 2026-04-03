package sync

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"

	_ "modernc.org/sqlite"
)

func TestParseOpenCodeDB(t *testing.T) {
	dbPath := createOpenCodeFixtureDB(t)
	sessions, err := ParseOpenCodeDB(dbPath)
	if err != nil {
		t.Fatalf("ParseOpenCodeDB() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	if sessions[0].Description != "Fallback first user prompt" {
		t.Fatalf("description = %q", sessions[0].Description)
	}
	if sessions[0].Cwd != "/Users/akoskm/Projects/other" {
		t.Fatalf("cwd = %q", sessions[0].Cwd)
	}
	if sessions[1].Description != "Go TUI for importing and assigning time entries" {
		t.Fatalf("fallback description = %q", sessions[1].Description)
	}
}

func TestImportOpenCodeLogs(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: model.CurrencyEUR})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.AddProjectPath(ctx, "hrs", "/Users/akoskm/Projects/hrs"); err != nil {
		t.Fatalf("AddProjectPath() error = %v", err)
	}
	dbPath := createOpenCodeFixtureDB(t)
	if err := ImportOpenCodeLogs(ctx, store, dbPath); err != nil {
		t.Fatalf("ImportOpenCodeLogs() error = %v", err)
	}
	if err := ImportOpenCodeLogs(ctx, store, dbPath); err != nil {
		t.Fatalf("ImportOpenCodeLogs() second run error = %v", err)
	}
	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	matched := 0
	for _, entry := range entries {
		if entry.Operator != opencodeSource {
			t.Fatalf("operator = %q, want %q", entry.Operator, opencodeSource)
		}
		if entry.Status != model.StatusDraft {
			t.Fatalf("status = %q, want %q", entry.Status, model.StatusDraft)
		}
		if entry.ProjectID != nil && *entry.ProjectID == project.ID {
			matched++
		}
	}
	if matched != 1 {
		t.Fatalf("matched = %d, want 1", matched)
	}
}

func TestImportOpenCodeLogsUpdatesExistingSession(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	dbPath := createOpenCodeFixtureDB(t)
	if err := ImportOpenCodeLogs(ctx, store, dbPath); err != nil {
		t.Fatalf("ImportOpenCodeLogs() error = %v", err)
	}
	if err := bumpOpenCodeSessionUpdatedAt(dbPath, "ses_1", 1775214583592); err != nil {
		t.Fatalf("bumpOpenCodeSessionUpdatedAt() error = %v", err)
	}
	if err := ImportOpenCodeLogs(ctx, store, dbPath); err != nil {
		t.Fatalf("ImportOpenCodeLogs() second run error = %v", err)
	}

	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		if entry.SourceRef != nil && *entry.SourceRef == "ses_1" {
			if entry.EndedAt == nil {
				t.Fatal("ended_at = nil, want value")
			}
			want := time.UnixMilli(1775214583592).UTC().Format(time.RFC3339)
			if entry.EndedAt.UTC().Format(time.RFC3339) != want {
				t.Fatalf("ended_at = %s, want %s", entry.EndedAt.UTC().Format(time.RFC3339), want)
			}
			return
		}
	}
	t.Fatal("updated session not found")
}

func createOpenCodeFixtureDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer conn.Close()
	stmts := []string{
		`CREATE TABLE session (id text PRIMARY KEY, project_id text NOT NULL, parent_id text, slug text NOT NULL, directory text NOT NULL, title text NOT NULL, version text NOT NULL, share_url text, summary_additions integer, summary_deletions integer, summary_files integer, summary_diffs text, revert text, permission text, time_created integer NOT NULL, time_updated integer NOT NULL, time_compacting integer, time_archived integer, workspace_id text);`,
		`CREATE TABLE message (id text PRIMARY KEY, session_id text NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL);`,
		`CREATE TABLE part (id text PRIMARY KEY, message_id text NOT NULL, session_id text NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL);`,
		`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated) VALUES ('ses_1', 'proj_1', 'one', '/Users/akoskm/Projects/hrs', 'Go TUI for importing and assigning time entries', '1', 1775194519167, 1775210983592);`,
		`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated) VALUES ('ses_2', 'proj_2', 'two', '/Users/akoskm/Projects/other', 'New session - 2026-04-03T00:00:00Z', '1', 1775190000000, 1775193600000);`,
		`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES ('msg_1', 'ses_2', 1775190001000, 1775190001000, '{"role":"user"}');`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES ('part_1', 'msg_1', 'ses_2', 1775190001001, 1775190001001, '{"type":"text","text":"Fallback first user prompt"}');`,
	}
	for _, stmt := range stmts {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("Exec(%q) error = %v", stmt, err)
		}
	}
	return path
}

func bumpOpenCodeSessionUpdatedAt(path, sessionID string, updatedAtMS int64) error {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Exec(`UPDATE session SET time_updated = ? WHERE id = ?`, updatedAtMS, sessionID)
	return err
}
