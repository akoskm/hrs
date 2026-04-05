package sync

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/akoskm/hrs/internal/db"

	_ "modernc.org/sqlite"
)

const opencodeSource = "opencode"

type OpenCodeSession struct {
	SessionID   string
	Description string
	StartedAt   time.Time
	EndedAt     time.Time
	Cwd         string
	SourcePath  string
}

// TODO(task5): rewrite to use activity slots
func ImportOpenCodeLogs(ctx context.Context, store *db.Store, dbPath string) error {
	return fmt.Errorf("opencode import not yet migrated to activity slots")
}

func ParseOpenCodeDB(path string) ([]OpenCodeSession, error) {
	dbConn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer dbConn.Close()

	rows, err := dbConn.Query(`
		SELECT id, directory, title, time_created, time_updated
		FROM session
		WHERE time_archived IS NULL
		ORDER BY time_created
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []OpenCodeSession
	for rows.Next() {
		var id, cwd, title string
		var createdMS, updatedMS int64
		if err := rows.Scan(&id, &cwd, &title, &createdMS, &updatedMS); err != nil {
			return nil, err
		}
		startedAt := time.UnixMilli(createdMS).UTC()
		endedAt := time.UnixMilli(updatedMS).UTC()
		if !endedAt.After(startedAt) {
			continue
		}
		description := strings.TrimSpace(title)
		if description == "" || strings.HasPrefix(description, "New session -") {
			description, err = firstOpenCodeUserText(dbConn, id)
			if err != nil {
				return nil, err
			}
		}
		if description == "" {
			description = id
		}
		sessions = append(sessions, OpenCodeSession{
			SessionID:   id,
			Description: normalizeDescription(description, 80),
			StartedAt:   startedAt,
			EndedAt:     endedAt,
			Cwd:         cwd,
			SourcePath:  path,
		})
	}
	return sessions, rows.Err()
}

func firstOpenCodeUserText(dbConn *sql.DB, sessionID string) (string, error) {
	rows, err := dbConn.Query(`
		SELECT p.data
		FROM part p
		JOIN message m ON m.id = p.message_id
		WHERE p.session_id = ?
		  AND json_extract(m.data, '$.role') = 'user'
		  AND json_extract(p.data, '$.type') = 'text'
		ORDER BY p.time_created ASC
	`, sessionID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return "", err
		}
		row := dbConn.QueryRow(`SELECT json_extract(?, '$.text')`, data)
		var text sql.NullString
		if err := row.Scan(&text); err != nil {
			return "", err
		}
		if strings.TrimSpace(text.String) != "" {
			return text.String, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func OpenCodeSummary(path string) string {
	return fmt.Sprintf("OpenCode DB: %s", path)
}
