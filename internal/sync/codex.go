package sync

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/akoskm/hrs/internal/db"
)

const codexSource = "codex"

type CodexSession struct {
	SessionID    string
	Description  string
	StartedAt    time.Time
	EndedAt      time.Time
	Cwd          string
	MessageCount int
	SourcePath   string
}

type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexSessionMeta struct {
	Cwd string `json:"cwd"`
}

type codexTurnContext struct {
	Cwd string `json:"cwd"`
}

type codexEventMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type codexResponseMessage struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

var cwdBlockRE = regexp.MustCompile(`(?s)<environment_context>.*?<cwd>(.*?)</cwd>.*?</environment_context>`)

func ParseCodexFile(path string) (CodexSession, error) {
	file, err := os.Open(path)
	if err != nil {
		return CodexSession{}, err
	}
	defer file.Close()

	var session CodexSession
	var first = true
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line codexLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return CodexSession{}, fmt.Errorf("parse %s: %w", path, err)
		}
		if line.Timestamp == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, line.Timestamp)
		if err != nil {
			return CodexSession{}, fmt.Errorf("parse timestamp %s: %w", path, err)
		}
		if first {
			session = CodexSession{SessionID: codexSessionIDFromPath(path), StartedAt: ts, EndedAt: ts, SourcePath: path}
			first = false
		}
		if ts.Before(session.StartedAt) {
			session.StartedAt = ts
		}
		if ts.After(session.EndedAt) {
			session.EndedAt = ts
		}

		switch line.Type {
		case "session_meta":
			var meta codexSessionMeta
			if err := json.Unmarshal(line.Payload, &meta); err == nil && session.Cwd == "" {
				session.Cwd = strings.TrimSpace(meta.Cwd)
			}
		case "turn_context":
			var tc codexTurnContext
			if err := json.Unmarshal(line.Payload, &tc); err == nil && session.Cwd == "" {
				session.Cwd = strings.TrimSpace(tc.Cwd)
			}
		case "event_msg":
			var ev codexEventMessage
			if err := json.Unmarshal(line.Payload, &ev); err == nil {
				if ev.Type == "user_message" {
					session.MessageCount++
					if session.Description == "" && strings.TrimSpace(ev.Message) != "" {
						session.Description = normalizeDescription(ev.Message, 80)
					}
				}
			}
		case "response_item":
			var msg codexResponseMessage
			if err := json.Unmarshal(line.Payload, &msg); err == nil {
				if msg.Type == "message" && msg.Role == "user" {
					for _, item := range msg.Content {
						if item.Type != "input_text" {
							continue
						}
						if session.Cwd == "" {
							if cwd := extractCodexCWD(item.Text); cwd != "" {
								session.Cwd = cwd
							}
						}
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return CodexSession{}, err
	}
	if first {
		return CodexSession{}, fmt.Errorf("%w in %s", ErrNoSessionData, path)
	}
	if session.Description == "" {
		session.Description = session.SessionID
	}
	if !session.EndedAt.After(session.StartedAt) {
		return CodexSession{}, fmt.Errorf("%w in %s", ErrSkipSession, path)
	}
	return session, nil
}

func ParseCodexTree(root string) ([]CodexSession, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasPrefix(base, "rollout-") || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	sessions := make([]CodexSession, 0, len(paths))
	for _, path := range paths {
		session, err := ParseCodexFile(path)
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// TODO(task5): rewrite to use activity slots
func ImportCodexLogs(ctx context.Context, store *db.Store, root string) error {
	return fmt.Errorf("codex import not yet migrated to activity slots")
}

func codexSessionIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func extractCodexCWD(text string) string {
	m := cwdBlockRE.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
