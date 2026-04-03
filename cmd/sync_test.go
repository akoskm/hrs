package cmd

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSyncCodexCommand(t *testing.T) {
	resetProjectCommandState()
	dbPath = t.TempDir() + "/hrs.db"
	fixturesPath = ""

	projectOut := &bytes.Buffer{}
	rootCmd.SetOut(projectOut)
	rootCmd.SetErr(projectOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "add", "wrkpad", "--code", "wrkpad"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project add execute error = %v", err)
	}

	pathOut := &bytes.Buffer{}
	rootCmd.SetOut(pathOut)
	rootCmd.SetErr(pathOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "path", "add", "/Users/akoskm/Projects/wrkpad", "wrkpad"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("path add execute error = %v", err)
	}

	syncOut := &bytes.Buffer{}
	rootCmd.SetOut(syncOut)
	rootCmd.SetErr(syncOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "sync", "codex", "--path", filepath.Join("..", "testdata", "codex-sessions")})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("sync codex execute error = %v", err)
	}
	if strings.Contains(syncOut.String(), "Error") {
		t.Fatalf("unexpected output = %q", syncOut.String())
	}
}

func TestSyncOpenCodeCommand(t *testing.T) {
	resetProjectCommandState()
	dbPath = t.TempDir() + "/hrs.db"
	fixturesPath = ""
	opencodePath := filepath.Join(t.TempDir(), "opencode.db")
	createSyncOpenCodeFixtureDB(t, opencodePath)

	projectOut := &bytes.Buffer{}
	rootCmd.SetOut(projectOut)
	rootCmd.SetErr(projectOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "add", "hrs", "--code", "hrs"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project add execute error = %v", err)
	}

	pathOut := &bytes.Buffer{}
	rootCmd.SetOut(pathOut)
	rootCmd.SetErr(pathOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "path", "add", "/Users/akoskm/Projects/hrs", "hrs"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("path add execute error = %v", err)
	}

	syncOut := &bytes.Buffer{}
	rootCmd.SetOut(syncOut)
	rootCmd.SetErr(syncOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "sync", "opencode", "--path", opencodePath})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("sync opencode execute error = %v", err)
	}
	if strings.Contains(syncOut.String(), "Error") {
		t.Fatalf("unexpected output = %q", syncOut.String())
	}
}

func createSyncOpenCodeFixtureDB(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer conn.Close()
	stmts := []string{
		`CREATE TABLE session (id text PRIMARY KEY, project_id text NOT NULL, parent_id text, slug text NOT NULL, directory text NOT NULL, title text NOT NULL, version text NOT NULL, share_url text, summary_additions integer, summary_deletions integer, summary_files integer, summary_diffs text, revert text, permission text, time_created integer NOT NULL, time_updated integer NOT NULL, time_compacting integer, time_archived integer, workspace_id text);`,
		`CREATE TABLE message (id text PRIMARY KEY, session_id text NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL);`,
		`CREATE TABLE part (id text PRIMARY KEY, message_id text NOT NULL, session_id text NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL);`,
		`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated) VALUES ('ses_sync', 'proj_1', 'sync', '/Users/akoskm/Projects/hrs', 'Go TUI for importing and assigning time entries', '1', 1775194519167, 1775210983592);`,
	}
	for _, stmt := range stmts {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("Exec(%q) error = %v", stmt, err)
		}
	}
}
