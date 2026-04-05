package db

import (
	"context"
	"testing"
	"time"

	"github.com/akoskm/hrs/internal/model"
)

func TestUpsertActivitySlotsAndList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	slotTime := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)
	slots := []model.ActivitySlot{
		{SlotTime: slotTime, Operator: "claude-code", Cwd: "/tmp/demo", MsgCount: 5, FirstText: "Fix auth"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	day := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	result, err := store.ListActivitySlotsForDay(ctx, day)
	if err != nil {
		t.Fatalf("ListActivitySlotsForDay() error = %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].MsgCount != 5 {
		t.Fatalf("MsgCount = %d, want 5", result[0].MsgCount)
	}

	// upsert again with updated msg_count
	slots[0].MsgCount = 10
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() second run error = %v", err)
	}
	result, err = store.ListActivitySlotsForDay(ctx, day)
	if err != nil {
		t.Fatalf("ListActivitySlotsForDay() error = %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len(result) = %d after upsert, want 1", len(result))
	}
	if result[0].MsgCount != 10 {
		t.Fatalf("MsgCount after upsert = %d, want 10", result[0].MsgCount)
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

func TestProjectGetsDefaultColorAndCanUpdateColor(t *testing.T) {
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
	if project.Color == nil || *project.Color == "" {
		t.Fatal("color = nil, want default color")
	}

	updated, err := store.UpdateProjectColorByID(ctx, project.ID, "#20c997")
	if err != nil {
		t.Fatalf("UpdateProjectColorByID() error = %v", err)
	}
	if updated.Color == nil || *updated.Color != "#20c997" {
		t.Fatalf("color = %v, want #20c997", updated.Color)
	}
}
