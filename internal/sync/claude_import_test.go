package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
)

func TestImportClaudeFixturesUpsertsActivitySlots(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	fixtures := filepath.Join("..", "..", "testdata", "claude-sessions")
	if err := ImportClaudeFixtures(ctx, store, fixtures); err != nil {
		t.Fatalf("ImportClaudeFixtures() error = %v", err)
	}
	// idempotent: running again should not fail
	if err := ImportClaudeFixtures(ctx, store, fixtures); err != nil {
		t.Fatalf("ImportClaudeFixtures() second run error = %v", err)
	}

	// sess_abc123 has timestamps spanning 10:00-11:00 -> 3 slots (10:00, 10:30, 11:00)
	// sess_def456 has timestamps spanning 10:15-11:45 -> 2 slots (10:15, 11:45)
	// total = 5 unique slots on 2026-04-03
	day := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	slots, err := store.ListActivitySlotsForDay(ctx, day)
	if err != nil {
		t.Fatalf("ListActivitySlotsForDay() error = %v", err)
	}
	if len(slots) != 5 {
		t.Fatalf("len(slots) = %d, want 5", len(slots))
	}
	for _, s := range slots {
		if s.Operator != "claude-code" {
			t.Fatalf("operator = %q, want claude-code", s.Operator)
		}
		if s.MsgCount < 1 {
			t.Fatalf("MsgCount = %d, want >= 1", s.MsgCount)
		}
	}
}

func TestImportClaudeLogsUpsertsSlots(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	root := t.TempDir()
	writeClaudeJSONL(t, filepath.Join(root, "work.jsonl"), []string{
		`{"sessionId":"work","timestamp":"2026-04-03T12:00:00Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"Fix report page"}}`,
		`{"sessionId":"work","timestamp":"2026-04-03T13:00:00Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":10,"output_tokens":20}}}`,
	})

	if err := ImportClaudeLogs(ctx, store, root); err != nil {
		t.Fatalf("ImportClaudeLogs() error = %v", err)
	}

	// 12:00 and 13:00 -> 2 slots
	day := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	slots, err := store.ListActivitySlotsForDay(ctx, day)
	if err != nil {
		t.Fatalf("ListActivitySlotsForDay() error = %v", err)
	}
	if len(slots) != 2 {
		t.Fatalf("len(slots) = %d, want 2", len(slots))
	}
	for _, s := range slots {
		if s.Operator != "claude-code" {
			t.Fatalf("operator = %q, want claude-code", s.Operator)
		}
	}
}

func TestImportClaudeLogsSkipsMalformedFiles(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	root := t.TempDir()
	// bad file: malformed JSON
	writeClaudeJSONL(t, filepath.Join(root, "bad.jsonl"), []string{`{bad json}`})
	// empty file: no data
	writeClaudeJSONL(t, filepath.Join(root, "empty.jsonl"), []string{})
	// good file
	writeClaudeJSONL(t, filepath.Join(root, "good.jsonl"), []string{
		`{"sessionId":"good","timestamp":"2026-04-03T09:00:00Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"fix auth module"}}`,
		`{"sessionId":"good","timestamp":"2026-04-03T09:20:00Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":10,"output_tokens":20}}}`,
	})

	if err := ImportClaudeLogs(ctx, store, root); err != nil {
		t.Fatalf("ImportClaudeLogs() error = %v", err)
	}

	day := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	slots, err := store.ListActivitySlotsForDay(ctx, day)
	if err != nil {
		t.Fatalf("ListActivitySlotsForDay() error = %v", err)
	}
	// 09:00 and 09:20 -> 09:00 and 09:15 buckets = 2 slots
	if len(slots) != 2 {
		t.Fatalf("len(slots) = %d, want 2", len(slots))
	}
}

func TestImportClaudeLogsIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	root := t.TempDir()
	writeClaudeJSONL(t, filepath.Join(root, "sess.jsonl"), []string{
		`{"sessionId":"idem","timestamp":"2026-04-03T14:00:00Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"fix auth module"}}`,
		`{"sessionId":"idem","timestamp":"2026-04-03T14:20:00Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":10,"output_tokens":20}}}`,
	})

	if err := ImportClaudeLogs(ctx, store, root); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	if err := ImportClaudeLogs(ctx, store, root); err != nil {
		t.Fatalf("second run error = %v", err)
	}

	day := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	slots, err := store.ListActivitySlotsForDay(ctx, day)
	if err != nil {
		t.Fatalf("ListActivitySlotsForDay() error = %v", err)
	}
	if len(slots) != 2 {
		t.Fatalf("len(slots) = %d, want 2", len(slots))
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

// Ensure model import is used
var _ = model.ActivitySlot{}
