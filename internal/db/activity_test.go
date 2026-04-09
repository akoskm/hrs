package db

import (
	"context"
	"testing"
	"time"

	"github.com/akoskm/hrs/internal/model"
)

func TestUpsertActivitySlots(t *testing.T) {
	t.Parallel()

	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 5, 8, 0, 0, 0, time.UTC), Operator: "claude-code", Cwd: "/tmp/project", MsgCount: 5, FirstText: "fix auth"},
		{SlotTime: time.Date(2026, 4, 5, 8, 15, 0, 0, time.UTC), Operator: "claude-code", Cwd: "/tmp/project", MsgCount: 3, FirstText: "add tests"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	// upsert again with higher count — should update
	slots[0].MsgCount = 10
	slots[0].FirstText = "fix auth better"
	slots[0].UserTexts = []string{"fix auth better", "add tests next"}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() second call error = %v", err)
	}

	got, err := store.ListActivitySlotsForDay(ctx, time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListActivitySlotsForDay() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d slots, want 2", len(got))
	}
	if got[0].MsgCount != 10 {
		t.Fatalf("msg_count = %d, want 10 after upsert", got[0].MsgCount)
	}
	if got[0].FirstText != "fix auth better" {
		t.Fatalf("first_text = %q, want updated value", got[0].FirstText)
	}
	if len(got[0].UserTexts) != 2 {
		t.Fatalf("len(user_texts) = %d, want 2", len(got[0].UserTexts))
	}
}

func TestListActivitySlotsForDay(t *testing.T) {
	t.Parallel()

	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 5, 8, 0, 0, 0, time.UTC), Operator: "claude-code", MsgCount: 1, FirstText: "work"},
		{SlotTime: time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC), Operator: "codex", MsgCount: 2, FirstText: "other work"},
		{SlotTime: time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC), Operator: "claude-code", MsgCount: 1, FirstText: "next day"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	got, err := store.ListActivitySlotsForDay(ctx, time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListActivitySlotsForDay() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d slots, want 2 (only Apr 5)", len(got))
	}
}
