package sync

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/akoskm/hrs/internal/model"

	_ "modernc.org/sqlite"
)

func LoadSessionDetail(entry model.TimeEntryDetail) ([]string, error) {
	if entry.Operator == "human" {
		return []string{"Manual entry", "No agent session attached."}, nil
	}
	md := entry.ParsedMetadata()
	sourcePath, _ := md["source_path"].(string)
	switch entry.Operator {
	case claudeSource:
		return loadClaudeSessionDetail(sourcePath)
	case codexSource:
		return loadCodexSessionDetail(sourcePath)
	case opencodeSource:
		if entry.SourceRef == nil {
			return nil, fmt.Errorf("missing source ref")
		}
		return loadOpenCodeSessionDetail(sourcePath, *entry.SourceRef)
	default:
		return []string{fmt.Sprintf("Source: %s", entry.Operator)}, nil
	}
}

func loadClaudeSessionDetail(path string) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("missing source path")
	}
	session, err := ParseClaudeSession(path)
	if err != nil {
		return nil, err
	}
	lines := []string{
		"Summary: " + session.Description,
		fmt.Sprintf("Messages: %d", session.MessageCount),
		"Path: " + path,
	}
	if session.FirstUserText != "" && session.FirstUserText != session.Description {
		lines = append(lines, "First prompt: "+session.FirstUserText)
	}
	if session.Cwd != "" {
		lines = append(lines, "CWD: "+session.Cwd)
	}
	if session.GitBranch != "" {
		lines = append(lines, "Branch: "+session.GitBranch)
	}
	return lines, nil
}

func loadCodexSessionDetail(path string) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("missing source path")
	}
	session, err := ParseCodexFile(path)
	if err != nil {
		return nil, err
	}
	lines := []string{
		"Summary: " + session.Description,
		fmt.Sprintf("Messages: %d", session.MessageCount),
		"Path: " + path,
	}
	if session.Cwd != "" {
		lines = append(lines, "CWD: "+session.Cwd)
	}
	return lines, nil
}

func loadOpenCodeSessionDetail(path, sessionID string) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("missing source path")
	}
	dbConn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer dbConn.Close()
	row := dbConn.QueryRow(`SELECT title, directory FROM session WHERE id = ?`, sessionID)
	var title string
	var cwd string
	if err := row.Scan(&title, &cwd); err != nil {
		return nil, err
	}
	firstPrompt, err := firstOpenCodeUserText(dbConn, sessionID)
	if err != nil {
		return nil, err
	}
	lines := []string{
		"Summary: " + normalizeDescription(title, 120),
		"Path: " + path,
	}
	if cwd != "" {
		lines = append(lines, "CWD: "+cwd)
	}
	if strings.TrimSpace(firstPrompt) != "" {
		lines = append(lines, "First prompt: "+normalizeDescription(firstPrompt, 120))
	}
	return lines, nil
}
