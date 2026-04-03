package sync

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/akoskm/hrs/internal/db"
)

const claudeSource = "claude-code"

type ClaudeSession struct {
	SessionID    string
	Description  string
	StartedAt    time.Time
	EndedAt      time.Time
	GitBranch    string
	Cwd          string
	MessageCount int
	SourcePath   string
}

type claudeMessage struct {
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	GitBranch string `json:"gitBranch"`
	Message   *struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
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
			for _, item := range msg.Message.Content {
				if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
					session.Description = strings.TrimSpace(item.Text)
					break
				}
			}
		}
		session.MessageCount++
	}
	if err := scanner.Err(); err != nil {
		return ClaudeSession{}, err
	}
	if first {
		return ClaudeSession{}, fmt.Errorf("no session data in %s", path)
	}
	if session.Description == "" {
		session.Description = session.SessionID
	}
	return session, nil
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
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func ImportClaudeFixtures(ctx context.Context, store *db.Store, dir string) error {
	sessions, err := ParseClaudeDir(dir)
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
		_, err = store.CreateImportedEntry(ctx, db.EntryImport{
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
