package sync

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
)

func TestParseCodexFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "testdata", "codex-sessions", "2026", "03", "30", "rollout-2026-03-30T11-23-39-019d3e79-f3b2-7722-9a42-565536435682.jsonl")
	session, err := ParseCodexFile(path)
	if err != nil {
		t.Fatalf("ParseCodexFile() error = %v", err)
	}
	if session.SessionID != "rollout-2026-03-30T11-23-39-019d3e79-f3b2-7722-9a42-565536435682" {
		t.Fatalf("session_id = %q", session.SessionID)
	}
	if session.Description != "panel don't exactly pop in light or dark mode. let's keep the design minimali..." {
		t.Fatalf("description = %q", session.Description)
	}
	if session.Cwd != "/Users/akoskm/Projects/wrkpad" {
		t.Fatalf("cwd = %q", session.Cwd)
	}
	if !session.EndedAt.After(session.StartedAt) {
		t.Fatalf("invalid range: %s - %s", session.StartedAt, session.EndedAt)
	}
}

func TestImportCodexLogs(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "wrkpad", Code: "wrkpad", Currency: model.CurrencyEUR})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.AddProjectPath(ctx, "wrkpad", "/Users/akoskm/Projects/wrkpad"); err != nil {
		t.Fatalf("AddProjectPath() error = %v", err)
	}
	root := filepath.Join("..", "..", "testdata", "codex-sessions")
	if err := ImportCodexLogs(ctx, store, root); err != nil {
		t.Fatalf("ImportCodexLogs() error = %v", err)
	}
	if err := ImportCodexLogs(ctx, store, root); err != nil {
		t.Fatalf("ImportCodexLogs() second run error = %v", err)
	}
	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Operator != codexSource {
		t.Fatalf("operator = %q, want %q", entry.Operator, codexSource)
	}
	if entry.Status != model.StatusDraft {
		t.Fatalf("status = %q, want %q", entry.Status, model.StatusDraft)
	}
	if entry.ProjectID == nil || *entry.ProjectID != project.ID {
		t.Fatalf("project_id = %v, want %q", entry.ProjectID, project.ID)
	}
	if entry.Description == nil || *entry.Description == "" {
		t.Fatalf("description = %v", entry.Description)
	}
}
