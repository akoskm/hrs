package sync

import (
	"errors"
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

func TestParseClaudeFileStringContent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "string-content.jsonl")
	content := `{"sessionId":"sess_string","timestamp":"2026-01-29T14:02:59.712Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"Find routes"}}` + "\n" +
		`{"sessionId":"sess_string","timestamp":"2026-01-29T14:12:59.712Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	session, err := ParseClaudeFile(path)
	if err != nil {
		t.Fatalf("ParseClaudeFile() error = %v", err)
	}
	if session.Description != "Find routes" {
		t.Fatalf("description = %q, want %q", session.Description, "Find routes")
	}
}

func TestParseClaudeFileTruncatesToFirstLineAnd80Chars(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "long-content.jsonl")
	content := `{"sessionId":"sess_long","timestamp":"2026-01-29T14:02:59.712Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"This is a very long first line that should be truncated after eighty characters exactly for display purposes.\nSecond line should be ignored."}}` + "\n" +
		`{"sessionId":"sess_long","timestamp":"2026-01-29T14:12:59.712Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	session, err := ParseClaudeFile(path)
	if err != nil {
		t.Fatalf("ParseClaudeFile() error = %v", err)
	}
	if got, want := session.Description, "This is a very long first line that should be truncated after eighty characte..."; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
}

func TestParseClaudeTreeSkipsUnreadableFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	goodPath := filepath.Join(root, "good.jsonl")
	badPath := filepath.Join(root, "bad.jsonl")
	goodContent := `{"sessionId":"sess_good","timestamp":"2026-01-29T14:02:59.712Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"Find routes"}}` + "\n" +
		`{"sessionId":"sess_good","timestamp":"2026-01-29T14:12:59.712Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}` + "\n"
	if err := os.WriteFile(goodPath, []byte(goodContent), 0o644); err != nil {
		t.Fatalf("WriteFile(good) error = %v", err)
	}
	if err := os.WriteFile(badPath, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile(bad) error = %v", err)
	}

	sessions, err := ParseClaudeTree(root)
	if err != nil {
		t.Fatalf("ParseClaudeTree() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	if sessions[0].SessionID != "sess_good" {
		t.Fatalf("session_id = %q, want %q", sessions[0].SessionID, "sess_good")
	}
}

func TestParseClaudeFileSkipsNoiseSession(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "noise.jsonl")
	content := `{"sessionId":"sess_noise","timestamp":"2026-01-29T14:02:59.712Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"<command-message>noise"}}` + "\n" +
		`{"sessionId":"sess_noise","timestamp":"2026-01-29T14:12:59.712Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := ParseClaudeFile(path); !errors.Is(err, ErrSkipSession) {
		t.Fatalf("ParseClaudeFile() error = %v, want ErrSkipSession", err)
	}
}

func TestParseClaudeFileSkipsZeroDuration(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "zero.jsonl")
	content := `{"sessionId":"sess_zero","timestamp":"2026-01-29T14:02:59.712Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"Real work"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := ParseClaudeFile(path); !errors.Is(err, ErrSkipSession) {
		t.Fatalf("ParseClaudeFile() error = %v, want ErrSkipSession", err)
	}
}
