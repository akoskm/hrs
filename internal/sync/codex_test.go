package sync

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/akoskm/hrs/internal/db"
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

func TestParseCodexSlots(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "testdata", "codex-sessions", "2026", "03", "30", "rollout-2026-03-30T11-23-39-019d3e79-f3b2-7722-9a42-565536435682.jsonl")
	slots, err := ParseCodexSlots(path)
	if err != nil {
		t.Fatalf("ParseCodexSlots() error = %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("expected at least one slot")
	}
	for _, slot := range slots {
		if slot.Operator != codexSource {
			t.Fatalf("operator = %q, want %q", slot.Operator, codexSource)
		}
		if slot.MsgCount == 0 {
			t.Fatal("msg_count = 0, want > 0")
		}
		// Verify slot time is rounded to 15 minutes
		if slot.SlotTime != slot.SlotTime.Truncate(15*time.Minute) {
			t.Fatalf("slot_time %s not rounded to 15 min", slot.SlotTime)
		}
	}
	// All messages in fixture are within 11:23-11:24, so one 11:15 slot
	if len(slots) != 1 {
		t.Fatalf("len(slots) = %d, want 1", len(slots))
	}
	if slots[0].Cwd != "/Users/akoskm/Projects/wrkpad" {
		t.Fatalf("cwd = %q", slots[0].Cwd)
	}
	if slots[0].FirstText == "" {
		t.Fatal("first_text is empty")
	}
}

func TestImportCodexLogs(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	root := filepath.Join("..", "..", "testdata", "codex-sessions")
	if err := ImportCodexLogs(ctx, store, root); err != nil {
		t.Fatalf("ImportCodexLogs() error = %v", err)
	}
	// Idempotent: second run should not error
	if err := ImportCodexLogs(ctx, store, root); err != nil {
		t.Fatalf("ImportCodexLogs() second run error = %v", err)
	}

	// Verify activity slots were created
	day := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	slots, err := store.ListActivitySlotsForDay(ctx, day)
	if err != nil {
		t.Fatalf("ListActivitySlotsForDay() error = %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("expected activity slots")
	}
	for _, slot := range slots {
		if slot.Operator != codexSource {
			t.Fatalf("operator = %q, want %q", slot.Operator, codexSource)
		}
		if slot.MsgCount == 0 {
			t.Fatal("msg_count = 0, want > 0")
		}
	}
}
