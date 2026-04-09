package sync

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestParseClaudeFileReturnsActivitySlots(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("..", "..", "testdata", "claude-sessions")

	// sess_abc123.jsonl has timestamps at 10:00, 10:00:15, 10:01, 10:05, 10:30, 10:30:10, 11:00
	// Expected 15-min buckets: 10:00, 10:30, 11:00
	slots, err := ParseClaudeFile(filepath.Join(dir, "sess_abc123.jsonl"))
	if err != nil {
		t.Fatalf("ParseClaudeFile() error = %v", err)
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].SlotTime.Before(slots[j].SlotTime) })

	if len(slots) != 3 {
		t.Fatalf("len(slots) = %d, want 3", len(slots))
	}

	want := []struct {
		slotTime time.Time
		operator string
		minMsgs  int
	}{
		{time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC), "claude-code", 1},
		{time.Date(2026, 4, 3, 10, 30, 0, 0, time.UTC), "claude-code", 1},
		{time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC), "claude-code", 1},
	}
	for i, w := range want {
		if !slots[i].SlotTime.Equal(w.slotTime) {
			t.Fatalf("slots[%d].SlotTime = %s, want %s", i, slots[i].SlotTime, w.slotTime)
		}
		if slots[i].Operator != w.operator {
			t.Fatalf("slots[%d].Operator = %q, want %q", i, slots[i].Operator, w.operator)
		}
		if slots[i].MsgCount < w.minMsgs {
			t.Fatalf("slots[%d].MsgCount = %d, want >= %d", i, slots[i].MsgCount, w.minMsgs)
		}
	}

	// first slot should capture the first user text
	if slots[0].FirstText == "" {
		t.Fatal("slots[0].FirstText is empty, want first user prompt")
	}
	// all slots should have cwd set
	for i, s := range slots {
		if s.Cwd == "" {
			t.Fatalf("slots[%d].Cwd is empty", i)
		}
	}
}

func TestParseClaudeFileWorktreeSession(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("..", "..", "testdata", "claude-sessions")

	// sess_def456.jsonl has timestamps at 10:15, 10:15:30, 11:45
	// Expected 15-min buckets: 10:15, 11:45
	slots, err := ParseClaudeFile(filepath.Join(dir, "sess_def456.jsonl"))
	if err != nil {
		t.Fatalf("ParseClaudeFile() error = %v", err)
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].SlotTime.Before(slots[j].SlotTime) })

	if len(slots) != 2 {
		t.Fatalf("len(slots) = %d, want 2", len(slots))
	}

	if !slots[0].SlotTime.Equal(time.Date(2026, 4, 3, 10, 15, 0, 0, time.UTC)) {
		t.Fatalf("slots[0].SlotTime = %s, want 2026-04-03T10:15:00Z", slots[0].SlotTime)
	}
	if !slots[1].SlotTime.Equal(time.Date(2026, 4, 3, 11, 45, 0, 0, time.UTC)) {
		t.Fatalf("slots[1].SlotTime = %s, want 2026-04-03T11:45:00Z", slots[1].SlotTime)
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
		`{"sessionId":"sess_string","timestamp":"2026-01-29T14:12:59.712Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":10,"output_tokens":20}}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	slots, err := ParseClaudeFile(path)
	if err != nil {
		t.Fatalf("ParseClaudeFile() error = %v", err)
	}
	// both timestamps (14:02 and 14:12) truncate to 14:00 -> 1 slot
	if len(slots) != 1 {
		t.Fatalf("len(slots) = %d, want 1", len(slots))
	}
	if slots[0].FirstText != "Find routes" {
		t.Fatalf("FirstText = %q, want %q", slots[0].FirstText, "Find routes")
	}
	if slots[0].MsgCount != 2 {
		t.Fatalf("MsgCount = %d, want 2", slots[0].MsgCount)
	}
}

func TestParseClaudeFileEmptyReturnsError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := ParseClaudeFile(path)
	if !errors.Is(err, ErrNoSessionData) {
		t.Fatalf("ParseClaudeFile() error = %v, want ErrNoSessionData", err)
	}
}

func TestParseClaudeFileSlotTimesRoundedTo15Min(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "rounding.jsonl")
	content := `{"sessionId":"sess_round","timestamp":"2026-01-29T14:07:30.000Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"user","content":"fix auth module"}}` + "\n" +
		`{"sessionId":"sess_round","timestamp":"2026-01-29T14:22:00.000Z","cwd":"/tmp/demo","gitBranch":"main","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":10,"output_tokens":20}}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	slots, err := ParseClaudeFile(path)
	if err != nil {
		t.Fatalf("ParseClaudeFile() error = %v", err)
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].SlotTime.Before(slots[j].SlotTime) })

	if len(slots) != 2 {
		t.Fatalf("len(slots) = %d, want 2", len(slots))
	}
	// 14:07:30 truncates to 14:00
	if !slots[0].SlotTime.Equal(time.Date(2026, 1, 29, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("slots[0].SlotTime = %s, want 2026-01-29T14:00:00Z", slots[0].SlotTime)
	}
	// 14:22:00 truncates to 14:15
	if !slots[1].SlotTime.Equal(time.Date(2026, 1, 29, 14, 15, 0, 0, time.UTC)) {
		t.Fatalf("slots[1].SlotTime = %s, want 2026-01-29T14:15:00Z", slots[1].SlotTime)
	}
}

func TestParseClaudeFileSkipsSystemOnlyNoiseSlots(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "noise.jsonl")
	content := `{"sessionId":"sess_noise","timestamp":"2026-04-08T12:21:03.416Z","cwd":"/tmp/demo","gitBranch":"dev","type":"system","subtype":"api_error"}` + "\n" +
		`{"sessionId":"sess_noise","timestamp":"2026-04-08T12:30:00.000Z","cwd":"/tmp/demo","gitBranch":"dev","message":{"role":"user","content":"fix auth flow"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	slots, err := ParseClaudeFile(path)
	if err != nil {
		t.Fatalf("ParseClaudeFile() error = %v", err)
	}
	if len(slots) != 1 {
		t.Fatalf("len(slots) = %d, want 1", len(slots))
	}
	if slots[0].FirstText != "fix auth flow" {
		t.Fatalf("FirstText = %q, want %q", slots[0].FirstText, "fix auth flow")
	}
}

func TestCleanPromptTextFiltersNoise(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"[Image: source: /Users/akoskm/.claude/image-cache/abc.png]", ""},
		{"[Image #14] some text", ""},
		{"<task-notification> stuff", ""},
		{"[Request interrupted by user]", ""},
		{"<system-reminder> blah", ""},
		{"short", ""},
		{"fix the authentication bug in login flow", "fix the authentication bug in login flow"},
		{"implement scroll [Image #5] in timeline", "implement scroll  in timeline"},
		{"okay commit this now", "okay commit this now"},
	}
	for _, tt := range tests {
		got := cleanPromptText(tt.input)
		if got != tt.want {
			t.Errorf("cleanPromptText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseClaudeFileCapturesBranchAndTokens(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "rich.jsonl")
	content := `{"sessionId":"s1","timestamp":"2026-01-29T14:00:00.000Z","cwd":"/tmp","gitBranch":"feat/auth","message":{"role":"user","content":"fix auth module","usage":{"input_tokens":500,"output_tokens":200}}}` + "\n" +
		`{"sessionId":"s1","timestamp":"2026-01-29T14:01:00.000Z","cwd":"/tmp","gitBranch":"feat/auth","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":1000,"output_tokens":800}}}` + "\n" +
		`{"sessionId":"s1","timestamp":"2026-01-29T14:02:00.000Z","cwd":"/tmp","gitBranch":"feat/auth","message":{"role":"user","content":"now add tests for it"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	slots, err := ParseClaudeFile(path)
	if err != nil {
		t.Fatalf("ParseClaudeFile() error = %v", err)
	}
	if len(slots) != 1 {
		t.Fatalf("len(slots) = %d, want 1", len(slots))
	}
	s := slots[0]
	if s.GitBranch != "feat/auth" {
		t.Errorf("GitBranch = %q, want feat/auth", s.GitBranch)
	}
	if s.TokenInput != 1500 {
		t.Errorf("TokenInput = %d, want 1500", s.TokenInput)
	}
	if s.TokenOutput != 1000 {
		t.Errorf("TokenOutput = %d, want 1000", s.TokenOutput)
	}
	if len(s.UserTexts) != 2 {
		t.Fatalf("len(UserTexts) = %d, want 2", len(s.UserTexts))
	}
	if s.UserTexts[0] != "fix auth module" {
		t.Errorf("UserTexts[0] = %q, want 'fix auth module'", s.UserTexts[0])
	}
	if s.UserTexts[1] != "now add tests for it" {
		t.Errorf("UserTexts[1] = %q, want 'now add tests for it'", s.UserTexts[1])
	}
}

func TestParseClaudeFileDeduplicatesPrompts(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "dedup.jsonl")
	content := `{"sessionId":"s1","timestamp":"2026-01-29T14:00:00.000Z","cwd":"/tmp","gitBranch":"main","message":{"role":"user","content":"fix the bug"}}` + "\n" +
		`{"sessionId":"s1","timestamp":"2026-01-29T14:00:30.000Z","cwd":"/tmp","gitBranch":"main","message":{"role":"user","content":"fix the bug"}}` + "\n" +
		`{"sessionId":"s1","timestamp":"2026-01-29T14:01:00.000Z","cwd":"/tmp","gitBranch":"main","message":{"role":"user","content":"actually add tests too"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	slots, err := ParseClaudeFile(path)
	if err != nil {
		t.Fatalf("ParseClaudeFile() error = %v", err)
	}
	if len(slots) != 1 {
		t.Fatalf("len(slots) = %d, want 1", len(slots))
	}
	if len(slots[0].UserTexts) != 2 {
		t.Fatalf("len(UserTexts) = %d, want 2 (deduped)", len(slots[0].UserTexts))
	}
}

func TestParseClaudeSessionStillWorks(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("..", "..", "testdata", "claude-sessions")
	session, err := ParseClaudeSession(filepath.Join(dir, "sess_abc123.jsonl"))
	if err != nil {
		t.Fatalf("ParseClaudeSession() error = %v", err)
	}
	if session.SessionID != "sess_abc123" {
		t.Fatalf("SessionID = %q, want %q", session.SessionID, "sess_abc123")
	}
	if session.MessageCount != 7 {
		t.Fatalf("MessageCount = %d, want 7", session.MessageCount)
	}
}
