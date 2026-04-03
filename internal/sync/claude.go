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
	"sort"
	"strings"
	"time"

	"github.com/akoskm/hrs/internal/db"
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
	Message   *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func ParseClaudeFile(path string) (ClaudeSession, error) {
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

func ParseClaudeDir(dir string) ([]ClaudeSession, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	sessions := make([]ClaudeSession, 0, len(paths))
	for _, path := range paths {
		session, err := ParseClaudeFile(path)
		if err != nil {
			if errors.Is(err, ErrSkipSession) {
				continue
			}
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func ParseClaudeTree(root string) ([]ClaudeSession, error) {
	var paths []string
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
		return nil, err
	}
	sort.Strings(paths)
	sessions := make([]ClaudeSession, 0, len(paths))
	for _, path := range paths {
		session, err := ParseClaudeFile(path)
		if err != nil {
			if errors.Is(err, ErrSkipSession) {
				continue
			}
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func ImportClaudeFixtures(ctx context.Context, store *db.Store, dir string) error {
	return importClaudeSessions(ctx, store, dir, ParseClaudeDir)
}

func ImportClaudeLogs(ctx context.Context, store *db.Store, root string) error {
	return importClaudeSessions(ctx, store, root, ParseClaudeTree)
}

func importClaudeSessions(ctx context.Context, store *db.Store, sourceRoot string, parse func(string) ([]ClaudeSession, error)) error {
	sessions, err := parse(sourceRoot)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		exists, err := store.HasImport(ctx, claudeSource, session.SessionID)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		projectID, err := store.DetectProjectIDByPath(ctx, session.Cwd)
		if err != nil {
			return err
		}
		overlap, err := store.HasOverlappingImportedEntry(ctx, session.Cwd, session.StartedAt, session.EndedAt, session.Description)
		if err != nil {
			return err
		}
		if overlap {
			continue
		}
		_, err = store.CreateImportedEntry(ctx, db.EntryImport{
			ProjectID:   projectID,
			Description: session.Description,
			StartedAt:   session.StartedAt,
			EndedAt:     session.EndedAt,
			Operator:    claudeSource,
			SourceRef:   session.SessionID,
			GitBranch:   session.GitBranch,
			Cwd:         session.Cwd,
			Metadata: map[string]any{
				"message_count": session.MessageCount,
				"source_path":   session.SourcePath,
			},
		})
		if err != nil {
			return err
		}
		if err := store.RecordImport(ctx, claudeSource, session.SourcePath, session.SessionID, 1); err != nil {
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
