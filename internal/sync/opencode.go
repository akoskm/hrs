package sync

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"

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

func ImportOpenCodeLogs(ctx context.Context, store *db.Store, dbPath string) error {
	dbConn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer dbConn.Close()

	// Query all message timestamps grouped by session, so we can bucket them
	rows, err := dbConn.Query(`
		SELECT s.directory, m.time_created, s.title, s.id, m.id, json_extract(m.data, '$.role')
		FROM message m
		JOIN session s ON s.id = m.session_id
		WHERE s.time_archived IS NULL
		ORDER BY m.time_created
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type slotKey struct {
		slotTime time.Time
		operator string
	}

	slots := make(map[slotKey]*model.ActivitySlot)

	for rows.Next() {
		var cwd, title, sessionID, messageID string
		var role sql.NullString
		var createdMS int64
		if err := rows.Scan(&cwd, &createdMS, &title, &sessionID, &messageID, &role); err != nil {
			return err
		}
		if strings.TrimSpace(role.String) != "user" {
			continue
		}
		text, err := openCodeMessageText(dbConn, messageID)
		if err != nil {
			return err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			text = openCodeSessionTitle(title)
		}
		if text == "" {
			continue
		}
		ts := time.UnixMilli(createdMS).UTC()
		bucket := ts.Truncate(15 * time.Minute)
		key := slotKey{slotTime: bucket, operator: opencodeSource}

		slot, ok := slots[key]
		if !ok {
			slot = &model.ActivitySlot{
				SlotTime: bucket,
				Operator: opencodeSource,
			}
			slots[key] = slot
		}
		slot.MsgCount++
		if slot.Cwd == "" {
			slot.Cwd = cwd
		}
		normalized := normalizeDescription(text, 80)
		if slot.FirstText == "" {
			slot.FirstText = normalized
		}
		slot.UserTexts = appendUniqueText(slot.UserTexts, normalized, 5)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(slots) == 0 {
		return nil
	}

	result := make([]model.ActivitySlot, 0, len(slots))
	for _, slot := range slots {
		result = append(result, *slot)
	}
	return store.UpsertActivitySlots(ctx, result)
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

func openCodeSessionTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" || strings.HasPrefix(title, "New session -") {
		return ""
	}
	return title
}

func openCodeMessageText(dbConn *sql.DB, messageID string) (string, error) {
	rows, err := dbConn.Query(`
		SELECT json_extract(data, '$.text')
		FROM part
		WHERE message_id = ?
		  AND json_extract(data, '$.type') = 'text'
		ORDER BY time_created ASC
	`, messageID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	parts := make([]string, 0, 2)
	for rows.Next() {
		var text sql.NullString
		if err := rows.Scan(&text); err != nil {
			return "", err
		}
		if strings.TrimSpace(text.String) != "" {
			parts = append(parts, strings.TrimSpace(text.String))
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(parts, "\n"), nil
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
