package sync

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
)

const claudeSource = "claude-code"

var ErrNoSessionData = errors.New("no session data")
var ErrSkipSession = errors.New("skip session")

var skippedDescriptionPrefixes = []string{
	"<local-command-caveat>",
	"<teammate-message",
	"[SUGGESTION MODE",
	"<command-message>",
}

type ClaudeSession struct {
	SessionID     string
	Description   string
	FirstUserText string
	StartedAt     time.Time
	EndedAt       time.Time
	GitBranch     string
	Cwd           string
	MessageCount  int
	SourcePath    string
}

type claudeSessionsIndex struct {
	Entries []struct {
		SessionID string `json:"sessionId"`
		Summary   string `json:"summary"`
	} `json:"entries"`
}

type claudeMessage struct {
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	GitBranch string `json:"gitBranch"`
	Message *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ParseClaudeFile parses a Claude Code JSONL file and returns activity slots
// bucketed into 15-minute intervals.
func ParseClaudeFile(path string) ([]model.ActivitySlot, error) {
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
		var msg claudeMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if msg.SessionID == "" || msg.Timestamp == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, msg.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp %s: %w", path, err)
		}
		hasData = true

		bucket := ts.Truncate(15 * time.Minute)
		key := slotKey{slotTime: bucket, operator: claudeSource}

		slot, ok := slots[key]
		if !ok {
			slot = &model.ActivitySlot{
				SlotTime: bucket,
				Operator: claudeSource,
			}
			slots[key] = slot
		}
		slot.MsgCount++
		if msg.Cwd != "" && slot.Cwd == "" {
			slot.Cwd = msg.Cwd
		}
		if msg.GitBranch != "" && slot.GitBranch == "" {
			slot.GitBranch = msg.GitBranch
		}
		if msg.Message != nil {
			if msg.Message.Usage != nil {
				slot.TokenInput += msg.Message.Usage.InputTokens
				slot.TokenOutput += msg.Message.Usage.OutputTokens
			}
			if msg.Message.Role == "user" {
				text, err := firstUserText(msg.Message.Content)
				if err != nil {
					return nil, fmt.Errorf("parse content %s: %w", path, err)
				}
				cleaned := cleanPromptText(text)
				if cleaned != "" {
					normalized := normalizeDescription(cleaned, 0)
					if slot.FirstText == "" {
						slot.FirstText = normalized
					}
					if len(slot.UserTexts) < 5 {
						// deduplicate
						dup := false
						for _, existing := range slot.UserTexts {
							if existing == normalized {
								dup = true
								break
							}
						}
						if !dup {
							slot.UserTexts = append(slot.UserTexts, normalized)
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

// ParseClaudeSession parses a Claude Code JSONL file and returns a ClaudeSession.
// Used by detail.go for session inspection.
func ParseClaudeSession(path string) (ClaudeSession, error) {
	file, err := os.Open(path)
	if err != nil {
		return ClaudeSession{}, err
	}
	defer file.Close()

	var session ClaudeSession
	var first bool = true
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var msg claudeMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return ClaudeSession{}, fmt.Errorf("parse %s: %w", path, err)
		}
		if msg.SessionID == "" || msg.Timestamp == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, msg.Timestamp)
		if err != nil {
			return ClaudeSession{}, fmt.Errorf("parse timestamp %s: %w", path, err)
		}
		if first {
			session = ClaudeSession{
				SessionID:  msg.SessionID,
				StartedAt:  ts,
				EndedAt:    ts,
				GitBranch:  msg.GitBranch,
				Cwd:        msg.Cwd,
				SourcePath: path,
			}
			first = false
		}
		if ts.Before(session.StartedAt) {
			session.StartedAt = ts
		}
		if ts.After(session.EndedAt) {
			session.EndedAt = ts
		}
		if session.Cwd == "" {
			session.Cwd = msg.Cwd
		}
		if session.GitBranch == "" {
			session.GitBranch = msg.GitBranch
		}
		if session.Description == "" && msg.Message != nil && msg.Message.Role == "user" {
			description, err := firstUserText(msg.Message.Content)
			if err != nil {
				return ClaudeSession{}, fmt.Errorf("parse content %s: %w", path, err)
			}
			if description != "" {
				session.Description = description
				session.FirstUserText = description
			}
		}
		session.MessageCount++
	}
	if err := scanner.Err(); err != nil {
		return ClaudeSession{}, err
	}
	if first {
		return ClaudeSession{}, fmt.Errorf("%w in %s", ErrNoSessionData, path)
	}
	if summary := lookupClaudeSummary(path, session.SessionID); summary != "" {
		session.Description = summary
	}
	if session.Description == "" {
		session.Description = session.SessionID
	}
	session.Description = normalizeDescription(session.Description, 80)
	if shouldSkipSession(session) {
		return ClaudeSession{}, fmt.Errorf("%w in %s", ErrSkipSession, path)
	}
	return session, nil
}

func lookupClaudeSummary(path, sessionID string) string {
	indexPath := filepath.Join(filepath.Dir(path), "sessions-index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return ""
	}
	var index claudeSessionsIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return ""
	}
	for _, entry := range index.Entries {
		if entry.SessionID == sessionID {
			return strings.TrimSpace(entry.Summary)
		}
	}
	return ""
}

func firstUserText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString), nil
	}
	var items []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", err
	}
	for _, item := range items {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			return strings.TrimSpace(item.Text), nil
		}
	}
	return "", nil
}

func normalizeDescription(text string, limit int) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(strings.Split(text, "\n")[0])
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return strings.TrimSpace(text[:limit-3]) + "..."
}

var noisePatterns = []string{
	"[Image: source:",
	"[Image #",
	"<task-notification>",
	"[Request interrupted by user]",
	"<local-command-",
	"<command-name>",
	"<system-reminder>",
	"<teammate-message",
}

func cleanPromptText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) < 10 {
		return ""
	}
	for _, pattern := range noisePatterns {
		if strings.HasPrefix(text, pattern) {
			return ""
		}
	}
	// strip inline image references
	for {
		start := strings.Index(text, "[Image")
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], "]")
		if end < 0 {
			break
		}
		text = text[:start] + text[start+end+1:]
	}
	return strings.TrimSpace(text)
}

func ImportClaudeFixtures(ctx context.Context, store *db.Store, dir string) error {
	return importClaudeSlots(ctx, store, dir, false)
}

func ImportClaudeLogs(ctx context.Context, store *db.Store, root string) error {
	return importClaudeSlots(ctx, store, root, true)
}

func importClaudeSlots(ctx context.Context, store *db.Store, root string, walk bool) error {
	var paths []string
	if walk {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".jsonl" {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		var err error
		paths, err = filepath.Glob(filepath.Join(root, "*.jsonl"))
		if err != nil {
			return err
		}
	}

	for _, path := range paths {
		slots, err := ParseClaudeFile(path)
		if err != nil {
			// skip unparseable files
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

func shouldSkipSession(session ClaudeSession) bool {
	if !session.EndedAt.After(session.StartedAt) {
		return true
	}
	probe := session.FirstUserText
	if probe == "" {
		probe = session.Description
	}
	for _, prefix := range skippedDescriptionPrefixes {
		if strings.HasPrefix(probe, prefix) {
			return true
		}
	}
	return false
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeCompareText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = nonWord.ReplaceAllString(text, " ")
	return strings.Join(strings.Fields(text), " ")
}

func SimilarDescription(a, b string) bool {
	a = normalizeCompareText(a)
	b = normalizeCompareText(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}
