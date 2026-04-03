package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/akoskm/hrs/internal/model"
	"github.com/oklog/ulid/v2"
)

type EntryImport struct {
	Description string
	StartedAt   time.Time
	EndedAt     time.Time
	Operator    string
	SourceRef   string
	GitBranch   string
	Cwd         string
	Metadata    map[string]any
}

func (s *Store) HasImport(ctx context.Context, source, sessionID string) (bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT 1 FROM import_log WHERE source = ? AND session_id = ?`, source, sessionID)
	var one int
	err := row.Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) CreateImportedEntry(ctx context.Context, entry EntryImport) (model.TimeEntry, error) {
	now := nowUTC()
	id := ulid.Make().String()
	duration := int(entry.EndedAt.Sub(entry.StartedAt).Seconds())
	metadataBytes, err := json.Marshal(entry.Metadata)
	if err != nil {
		return model.TimeEntry{}, err
	}
	metadata := string(metadataBytes)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO time_entries (
			id, description, started_at, ended_at, duration_secs, billable, status, operator,
			source_ref, git_branch, cwd, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		id,
		entry.Description,
		entry.StartedAt.UTC().Format(timeFormat),
		entry.EndedAt.UTC().Format(timeFormat),
		duration,
		model.StatusDraft,
		entry.Operator,
		entry.SourceRef,
		entry.GitBranch,
		entry.Cwd,
		metadata,
		now.Format(timeFormat),
		now.Format(timeFormat),
	)
	if err != nil {
		return model.TimeEntry{}, err
	}
	return s.EntryByID(ctx, id)
}

func (s *Store) RecordImport(ctx context.Context, source, sourcePath, sessionID string, entriesCreated int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO import_log (id, source, source_path, session_id, entries_created)
		VALUES (?, ?, ?, ?, ?)
	`, ulid.Make().String(), source, sourcePath, sessionID, entriesCreated)
	return err
}

func (s *Store) ListEntries(ctx context.Context) ([]model.TimeEntryDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			te.id, te.project_id, te.task_id, te.description, te.started_at, te.ended_at, te.duration_secs,
			te.billable, te.status, te.operator, te.source_ref, te.worktree, te.git_branch, te.cwd,
			te.metadata, te.deleted_at, te.created_at, te.updated_at,
			COALESCE(p.name, ''), p.code
		FROM time_entries te
		LEFT JOIN projects p ON p.id = te.project_id
		WHERE te.deleted_at IS NULL
		ORDER BY te.started_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.TimeEntryDetail
	for rows.Next() {
		entry, err := scanTimeEntryDetail(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) EntryByID(ctx context.Context, id string) (model.TimeEntry, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, task_id, description, started_at, ended_at, duration_secs,
			billable, status, operator, source_ref, worktree, git_branch, cwd, metadata,
			deleted_at, created_at, updated_at
		FROM time_entries WHERE id = ?
	`, id)
	return scanTimeEntry(row)
}

func (s *Store) AssignEntryToProject(ctx context.Context, entryID, projectID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE time_entries
		SET project_id = ?, status = ?, updated_at = ?
		WHERE id = ?
	`, projectID, model.StatusConfirmed, nowUTC().Format(timeFormat), entryID)
	return err
}

type entryScanner interface {
	Scan(dest ...any) error
}

func scanTimeEntry(row entryScanner) (model.TimeEntry, error) {
	var entry model.TimeEntry
	var projectID sql.NullString
	var taskID sql.NullString
	var description sql.NullString
	var startedAt string
	var endedAt sql.NullString
	var duration sql.NullInt64
	var sourceRef sql.NullString
	var worktree sql.NullString
	var gitBranch sql.NullString
	var cwd sql.NullString
	var metadata sql.NullString
	var deletedAt sql.NullString
	var createdAt string
	var updatedAt string
	var billable int
	if err := row.Scan(
		&entry.ID,
		&projectID,
		&taskID,
		&description,
		&startedAt,
		&endedAt,
		&duration,
		&billable,
		&entry.Status,
		&entry.Operator,
		&sourceRef,
		&worktree,
		&gitBranch,
		&cwd,
		&metadata,
		&deletedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.TimeEntry{}, err
	}

	entry.ProjectID = scanNullString(projectID)
	entry.TaskID = scanNullString(taskID)
	entry.Description = scanNullString(description)
	entry.SourceRef = scanNullString(sourceRef)
	entry.Worktree = scanNullString(worktree)
	entry.GitBranch = scanNullString(gitBranch)
	entry.Cwd = scanNullString(cwd)
	entry.Metadata = scanNullString(metadata)
	entry.Billable = billable == 1
	if duration.Valid {
		v := int(duration.Int64)
		entry.DurationSecs = &v
	}

	var err error
	entry.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return model.TimeEntry{}, err
	}
	entry.EndedAt, err = scanNullTime(endedAt)
	if err != nil {
		return model.TimeEntry{}, err
	}
	entry.DeletedAt, err = scanNullTime(deletedAt)
	if err != nil {
		return model.TimeEntry{}, err
	}
	entry.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.TimeEntry{}, err
	}
	entry.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.TimeEntry{}, err
	}
	return entry, nil
}

func scanTimeEntryDetail(row entryScanner) (model.TimeEntryDetail, error) {
	var detail model.TimeEntryDetail
	var projectID sql.NullString
	var taskID sql.NullString
	var description sql.NullString
	var startedAt string
	var endedAt sql.NullString
	var duration sql.NullInt64
	var sourceRef sql.NullString
	var worktree sql.NullString
	var gitBranch sql.NullString
	var cwd sql.NullString
	var metadata sql.NullString
	var deletedAt sql.NullString
	var createdAt string
	var updatedAt string
	var projectCode sql.NullString
	var taskName sql.NullString
	var clientName sql.NullString
	var billable int
	if err := row.Scan(
		&detail.ID,
		&projectID,
		&taskID,
		&description,
		&startedAt,
		&endedAt,
		&duration,
		&billable,
		&detail.Status,
		&detail.Operator,
		&sourceRef,
		&worktree,
		&gitBranch,
		&cwd,
		&metadata,
		&deletedAt,
		&createdAt,
		&updatedAt,
		&detail.ProjectName,
		&projectCode,
	); err != nil {
		return model.TimeEntryDetail{}, err
	}

	detail.ProjectID = scanNullString(projectID)
	detail.TaskID = scanNullString(taskID)
	detail.Description = scanNullString(description)
	detail.SourceRef = scanNullString(sourceRef)
	detail.Worktree = scanNullString(worktree)
	detail.GitBranch = scanNullString(gitBranch)
	detail.Cwd = scanNullString(cwd)
	detail.Metadata = scanNullString(metadata)
	detail.ProjectCode = scanNullString(projectCode)
	detail.TaskName = scanNullString(taskName)
	detail.ClientName = scanNullString(clientName)
	detail.Billable = billable == 1
	if duration.Valid {
		v := int(duration.Int64)
		detail.DurationSecs = &v
	}

	var err error
	detail.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return model.TimeEntryDetail{}, err
	}
	detail.EndedAt, err = scanNullTime(endedAt)
	if err != nil {
		return model.TimeEntryDetail{}, err
	}
	detail.DeletedAt, err = scanNullTime(deletedAt)
	if err != nil {
		return model.TimeEntryDetail{}, err
	}
	detail.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.TimeEntryDetail{}, err
	}
	detail.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.TimeEntryDetail{}, err
	}
	return detail, nil
}
