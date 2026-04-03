package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseClaudeDir(t *testing.T) {
	t.Parallel()

	sessions, err := ParseClaudeDir(filepath.Join("..", "..", "testdata", "claude-sessions"))
	if err != nil {
		t.Fatalf("ParseClaudeDir() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}

	tests := map[string]struct {
		description string
		startedAt   string
		endedAt     string
		messages    int
	}{
		"sess_abc123": {
			description: "Refactor the auth module to use OAuth2",
			startedAt:   "2026-04-03T10:00:00Z",
			endedAt:     "2026-04-03T11:00:00Z",
			messages:    7,
		},
		"sess_def456": {
			description: "Implement the email automation workflow with Inngest",
			startedAt:   "2026-04-03T10:15:00Z",
			endedAt:     "2026-04-03T11:45:00Z",
			messages:    3,
		},
	}

	for _, session := range sessions {
		want, ok := tests[session.SessionID]
		if !ok {
			t.Fatalf("unexpected session %q", session.SessionID)
		}
		if session.Description != want.description {
			t.Fatalf("description = %q, want %q", session.Description, want.description)
		}
		startedAt, _ := time.Parse(time.RFC3339, want.startedAt)
		endedAt, _ := time.Parse(time.RFC3339, want.endedAt)
		if !session.StartedAt.Equal(startedAt) {
			t.Fatalf("started_at = %s, want %s", session.StartedAt, startedAt)
		}
		if !session.EndedAt.Equal(endedAt) {
			t.Fatalf("ended_at = %s, want %s", session.EndedAt, endedAt)
		}
		if session.MessageCount != want.messages {
			t.Fatalf("message_count = %d, want %d", session.MessageCount, want.messages)
		}
	}
}

func TestParseClaudeFileMalformed(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(path, []byte("{bad json}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := ParseClaudeFile(path); err == nil {
		t.Fatal("ParseClaudeFile() error = nil, want error")
	}
}
