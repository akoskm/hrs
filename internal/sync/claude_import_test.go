package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
)

func TestImportClaudeFixturesDetectsProjectAndSkipsDuplicates(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: model.CurrencyCHF})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.AddProjectPath(ctx, "elaiia", "/Users/akos/code/elaiia"); err != nil {
		t.Fatalf("AddProjectPath() error = %v", err)
	}

	fixtures := filepath.Join("..", "..", "testdata", "claude-sessions")
	if err := ImportClaudeFixtures(ctx, store, fixtures); err != nil {
		t.Fatalf("ImportClaudeFixtures() error = %v", err)
	}
	if err := ImportClaudeFixtures(ctx, store, fixtures); err != nil {
		t.Fatalf("ImportClaudeFixtures() second run error = %v", err)
	}

	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		if entry.Status != model.StatusDraft {
			t.Fatalf("status = %q, want %q", entry.Status, model.StatusDraft)
		}
		if entry.ProjectID == nil || *entry.ProjectID != project.ID {
			t.Fatalf("project_id = %v, want %q", entry.ProjectID, project.ID)
		}
	}
}

func TestImportClaudeLogsSkipsNoiseAndZeroDuration(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Delta", Code: "delta", Currency: model.CurrencyEUR})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.AddProjectPath(ctx, "delta", "/Users/akoskm/Projects/delta-one-nextjs"); err != nil {
		t.Fatalf("AddProjectPath() error = %v", err)
	}

	root := t.TempDir()
	writeClaudeJSONL(t, filepath.Join(root, "noise.jsonl"), []string{
		`{"sessionId":"noise","timestamp":"2026-04-03T10:00:00Z","cwd":"/Users/akoskm/Projects/delta-one-nextjs","gitBranch":"main","message":{"role":"user","content":"<local-command-caveat>Caveat text"}}`,
		`{"sessionId":"noise","timestamp":"2026-04-03T10:30:00Z","cwd":"/Users/akoskm/Projects/delta-one-nextjs","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}`,
	})
	writeClaudeJSONL(t, filepath.Join(root, "zero.jsonl"), []string{
		`{"sessionId":"zero","timestamp":"2026-04-03T11:00:00Z","cwd":"/Users/akoskm/Projects/delta-one-nextjs","gitBranch":"main","message":{"role":"user","content":"real work"}}`,
	})
	writeClaudeJSONL(t, filepath.Join(root, "worktree.jsonl"), []string{
		`{"sessionId":"worktree","timestamp":"2026-04-03T12:00:00Z","cwd":"/Users/akoskm/Projects/delta-one-nextjs/.claude/worktrees/foo","gitBranch":"feat/x","message":{"role":"user","content":"Fix report page"}}`,
		`{"sessionId":"worktree","timestamp":"2026-04-03T13:00:00Z","cwd":"/Users/akoskm/Projects/delta-one-nextjs/.claude/worktrees/foo","gitBranch":"feat/x","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`,
	})

	if err := ImportClaudeLogs(ctx, store, root); err != nil {
		t.Fatalf("ImportClaudeLogs() error = %v", err)
	}

	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if got := entries[0].Description; got == nil || *got != "Fix report page" {
		t.Fatalf("description = %v, want Fix report page", got)
	}
	if entries[0].ProjectID == nil || *entries[0].ProjectID != project.ID {
		t.Fatalf("project_id = %v, want %q", entries[0].ProjectID, project.ID)
	}
	if entries[0].Status != model.StatusDraft {
		t.Fatalf("status = %q, want %q", entries[0].Status, model.StatusDraft)
	}
	noiseImported, err := store.HasImport(ctx, claudeSource, "noise")
	if err != nil {
		t.Fatalf("HasImport(noise) error = %v", err)
	}
	if noiseImported {
		t.Fatal("noise session should not be recorded in import_log")
	}
}

func TestImportClaudeLogsSkipsOverlapDuplicates(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	root := t.TempDir()
	writeClaudeJSONL(t, filepath.Join(root, "one.jsonl"), []string{
		`{"sessionId":"one","timestamp":"2026-04-03T10:00:00Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"Fix report page"}}`,
		`{"sessionId":"one","timestamp":"2026-04-03T11:00:00Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`,
	})
	writeClaudeJSONL(t, filepath.Join(root, "two.jsonl"), []string{
		`{"sessionId":"two","timestamp":"2026-04-03T10:05:00Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"Fix report page now"}}`,
		`{"sessionId":"two","timestamp":"2026-04-03T10:59:00Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`,
	})

	if err := ImportClaudeLogs(ctx, store, root); err != nil {
		t.Fatalf("ImportClaudeLogs() error = %v", err)
	}
	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
}

func writeClaudeJSONL(t *testing.T, path string, lines []string) {
	t.Helper()
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
