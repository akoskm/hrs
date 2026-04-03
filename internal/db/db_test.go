package db

import (
	"context"
	"testing"
	"time"

	"github.com/akoskm/hrs/internal/model"
)

func TestCreateImportedEntryAndAssign(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	project, err := store.EnsureProject(ctx, "Elaiia", "elaiia")
	if err != nil {
		t.Fatalf("EnsureProject() error = %v", err)
	}

	startedAt := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(time.Hour)
	entry, err := store.CreateImportedEntry(ctx, EntryImport{
		Description: "OAuth2 refactor",
		StartedAt:   startedAt,
		EndedAt:     endedAt,
		Operator:    "claude-code",
		SourceRef:   "sess_abc123",
		GitBranch:   "feat/auth-refactor",
		Cwd:         "/Users/akos/code/elaiia",
		Metadata:    map[string]any{"message_count": 7},
	})
	if err != nil {
		t.Fatalf("CreateImportedEntry() error = %v", err)
	}
	if entry.Status != model.StatusDraft {
		t.Fatalf("status = %q, want %q", entry.Status, model.StatusDraft)
	}
	if entry.ProjectID != nil {
		t.Fatalf("project_id = %v, want nil", entry.ProjectID)
	}

	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	if err := store.AssignEntryToProject(ctx, entry.ID, project.ID); err != nil {
		t.Fatalf("AssignEntryToProject() error = %v", err)
	}

	updated, err := store.EntryByID(ctx, entry.ID)
	if err != nil {
		t.Fatalf("EntryByID() error = %v", err)
	}
	if updated.Status != model.StatusConfirmed {
		t.Fatalf("status = %q, want %q", updated.Status, model.StatusConfirmed)
	}
	if updated.ProjectID == nil || *updated.ProjectID != project.ID {
		t.Fatalf("project_id = %v, want %q", updated.ProjectID, project.ID)
	}
}

func TestProjectUpdateAndArchiveByID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	project, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: model.CurrencyCHF})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	updated, err := store.UpdateProjectBillableDefaultByID(ctx, project.ID, false)
	if err != nil {
		t.Fatalf("UpdateProjectBillableDefaultByID() error = %v", err)
	}
	if updated.BillableDefault {
		t.Fatal("billable_default = true, want false")
	}

	if err := store.ArchiveProjectByID(ctx, project.ID); err != nil {
		t.Fatalf("ArchiveProjectByID() error = %v", err)
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("len(projects) = %d, want 0", len(projects))
	}

	archived, err := store.ProjectByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("ProjectByID() error = %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("archived_at = nil, want value")
	}
}
