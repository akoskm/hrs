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
	"github.com/akoskm/hrs/internal/model"
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

// ParseCodexSlots parses a Codex JSONL file and returns activity slots
// bucketed into 15-minute intervals.
func ParseCodexSlots(path string) ([]model.ActivitySlot, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	type slotKey struct {
		slotTime time.Time
		operator string
	}

	slots := make(map[slotKey]*model.ActivitySlot)
	var hasData bool

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line codexLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if line.Timestamp == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, line.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp %s: %w", path, err)
		}
		hasData = true

		bucket := ts.Truncate(15 * time.Minute)
		key := slotKey{slotTime: bucket, operator: codexSource}

		slot, ok := slots[key]
		if !ok {
			slot = &model.ActivitySlot{
				SlotTime: bucket,
				Operator: codexSource,
			}
			slots[key] = slot
		}
		slot.MsgCount++

		// Extract cwd from session_meta or turn_context
		switch line.Type {
		case "session_meta":
			var meta codexSessionMeta
			if err := json.Unmarshal(line.Payload, &meta); err == nil && slot.Cwd == "" {
				slot.Cwd = strings.TrimSpace(meta.Cwd)
			}
		case "turn_context":
			var tc codexTurnContext
			if err := json.Unmarshal(line.Payload, &tc); err == nil && slot.Cwd == "" {
				slot.Cwd = strings.TrimSpace(tc.Cwd)
			}
		case "event_msg":
			var ev codexEventMessage
			if err := json.Unmarshal(line.Payload, &ev); err == nil {
				if ev.Type == "user_message" && slot.FirstText == "" && strings.TrimSpace(ev.Message) != "" {
					slot.FirstText = normalizeDescription(ev.Message, 80)
				}
			}
		case "response_item":
			var msg codexResponseMessage
			if err := json.Unmarshal(line.Payload, &msg); err == nil {
				if msg.Type == "message" && msg.Role == "user" && slot.Cwd == "" {
					for _, item := range msg.Content {
						if item.Type == "input_text" {
							if cwd := extractCodexCWD(item.Text); cwd != "" {
								slot.Cwd = cwd
							}
						}
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !hasData {
		return nil, fmt.Errorf("%w in %s", ErrNoSessionData, path)
	}

	result := make([]model.ActivitySlot, 0, len(slots))
	for _, slot := range slots {
		result = append(result, *slot)
	}
	return result, nil
}

func ImportCodexLogs(ctx context.Context, store *db.Store, root string) error {
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
		return err
	}

	for _, path := range paths {
		slots, err := ParseCodexSlots(path)
		if err != nil {
			continue
		}
		if len(slots) == 0 {
			continue
		}
		if err := store.UpsertActivitySlots(ctx, slots); err != nil {
			return err
		}
	}
	return nil
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
