package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/akoskm/hrs/internal/model"
	"github.com/oklog/ulid/v2"
)

type EntryImport struct {
	ProjectID   *string
	Description string
	StartedAt   time.Time
	EndedAt     time.Time
	Operator    string
	SourceRef   string
	GitBranch   string
	Cwd         string
	Metadata    map[string]any
}

type ManualEntryInput struct {
	ProjectIdent string
	Description  string
	StartedAt    time.Time
	EndedAt      time.Time
	Billable     *bool
}

type AgentEntryUpsertInput struct {
	ProjectIdent string
	Description  string
	StartedAt    time.Time
	EndedAt      time.Time
	Operator     string
	SourceRef    string
	GitBranch    string
	Cwd          string
	Metadata     map[string]any
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
	var projectID any
	if entry.ProjectID != nil {
		projectID = *entry.ProjectID
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO time_entries (
			id, project_id, description, started_at, ended_at, duration_secs, billable, status, operator,
			source_ref, git_branch, cwd, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		id,
		projectID,
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

func (s *Store) UpdateImportedEntryBySourceRef(ctx context.Context, operator, sourceRef string, entry EntryImport) (model.TimeEntry, error) {
	now := nowUTC()
	duration := int(entry.EndedAt.Sub(entry.StartedAt).Seconds())
	metadataBytes, err := json.Marshal(entry.Metadata)
	if err != nil {
		return model.TimeEntry{}, err
	}
	metadata := string(metadataBytes)
	var projectID any
	if entry.ProjectID != nil {
		projectID = *entry.ProjectID
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE time_entries
		SET project_id = ?, description = ?, started_at = ?, ended_at = ?, duration_secs = ?, cwd = ?, metadata = ?, updated_at = ?
		WHERE operator = ? AND source_ref = ? AND deleted_at IS NULL
	`, projectID, entry.Description, entry.StartedAt.UTC().Format(timeFormat), entry.EndedAt.UTC().Format(timeFormat), duration, entry.Cwd, metadata, now.Format(timeFormat), operator, sourceRef)
	if err != nil {
		return model.TimeEntry{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return model.TimeEntry{}, err
	}
	if rows == 0 {
		return model.TimeEntry{}, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, task_id, description, started_at, ended_at, duration_secs,
			billable, status, operator, source_ref, worktree, git_branch, cwd, metadata,
			deleted_at, created_at, updated_at
		FROM time_entries WHERE operator = ? AND source_ref = ? AND deleted_at IS NULL
	`, operator, sourceRef)
	return scanTimeEntry(row)
}

func (s *Store) CreateManualEntry(ctx context.Context, entry ManualEntryInput) (model.TimeEntry, error) {
	project, err := s.ProjectByCodeOrName(ctx, entry.ProjectIdent)
	if err != nil {
		return model.TimeEntry{}, err
	}
	now := nowUTC()
	id := ulid.Make().String()
	duration := int(entry.EndedAt.Sub(entry.StartedAt).Seconds())
	billable := project.BillableDefault
	if entry.Billable != nil {
		billable = *entry.Billable
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO time_entries (
			id, project_id, description, started_at, ended_at, duration_secs, billable, status, operator, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'human', ?, ?)
	`,
		id,
		project.ID,
		entry.Description,
		entry.StartedAt.UTC().Format(timeFormat),
		entry.EndedAt.UTC().Format(timeFormat),
		duration,
		boolToInt(billable),
		model.StatusConfirmed,
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
			COALESCE(p.name, ''), p.code, p.color
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

func (s *Store) UnassignEntryProject(ctx context.Context, entryID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE time_entries
		SET project_id = NULL, status = ?, updated_at = ?
		WHERE id = ?
	`, model.StatusDraft, nowUTC().Format(timeFormat), entryID)
	return err
}

func (s *Store) UpdateEntryDescriptionAndProject(ctx context.Context, entryID string, description string, projectID *string) error {
	status := model.StatusDraft
	var dbProjectID any
	if projectID != nil {
		status = model.StatusConfirmed
		dbProjectID = *projectID
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE time_entries
		SET description = ?, project_id = ?, status = ?, updated_at = ?
		WHERE id = ?
	`, description, dbProjectID, status, nowUTC().Format(timeFormat), entryID)
	return err
}

func (s *Store) UpdateEntry(ctx context.Context, entryID string, description string, projectID *string, startedAt, endedAt time.Time) error {
	status := model.StatusDraft
	var dbProjectID any
	if projectID != nil {
		status = model.StatusConfirmed
		dbProjectID = *projectID
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE time_entries
		SET description = ?, project_id = ?, started_at = ?, ended_at = ?, duration_secs = ?, status = ?, updated_at = ?
		WHERE id = ?
	`, description, dbProjectID, startedAt.UTC().Format(timeFormat), endedAt.UTC().Format(timeFormat), int(endedAt.Sub(startedAt).Seconds()), status, nowUTC().Format(timeFormat), entryID)
	return err
}

func (s *Store) UpsertAgentEntry(ctx context.Context, input AgentEntryUpsertInput) (model.TimeEntry, error) {
	now := nowUTC()
	duration := int(input.EndedAt.Sub(input.StartedAt).Seconds())
	metadataBytes, err := json.Marshal(input.Metadata)
	if err != nil {
		return model.TimeEntry{}, err
	}
	metadata := string(metadataBytes)
	var projectID any
	status := model.StatusDraft
	billable := 1
	if strings.TrimSpace(input.ProjectIdent) != "" {
		project, err := s.ProjectByCodeOrName(ctx, strings.TrimSpace(input.ProjectIdent))
		if err != nil {
			return model.TimeEntry{}, err
		}
		projectID = project.ID
		status = model.StatusConfirmed
		if !project.BillableDefault {
			billable = 0
		}
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id FROM time_entries WHERE operator = ? AND source_ref = ? AND deleted_at IS NULL
	`, input.Operator, input.SourceRef)
	var existingID string
	if err := row.Scan(&existingID); err != nil {
		if err != sql.ErrNoRows {
			return model.TimeEntry{}, err
		}
		id := ulid.Make().String()
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO time_entries (
				id, project_id, description, started_at, ended_at, duration_secs, billable, status, operator,
				source_ref, git_branch, cwd, metadata, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, projectID, input.Description, input.StartedAt.UTC().Format(timeFormat), input.EndedAt.UTC().Format(timeFormat), duration, billable, status, input.Operator, input.SourceRef, input.GitBranch, input.Cwd, metadata, now.Format(timeFormat), now.Format(timeFormat))
		if err != nil {
			return model.TimeEntry{}, err
		}
		return s.EntryByID(ctx, id)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE time_entries
		SET project_id = ?, description = ?, started_at = ?, ended_at = ?, duration_secs = ?, billable = ?, status = ?, git_branch = ?, cwd = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, projectID, input.Description, input.StartedAt.UTC().Format(timeFormat), input.EndedAt.UTC().Format(timeFormat), duration, billable, status, input.GitBranch, input.Cwd, metadata, now.Format(timeFormat), existingID)
	if err != nil {
		return model.TimeEntry{}, err
	}
	return s.EntryByID(ctx, existingID)
}

func (s *Store) SoftDeleteEntry(ctx context.Context, entryID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE time_entries SET deleted_at = ?, updated_at = ? WHERE id = ?
	`, nowUTC().Format(timeFormat), nowUTC().Format(timeFormat), entryID)
	return err
}

func (s *Store) HasOverlappingImportedEntry(ctx context.Context, cwd string, startedAt, endedAt time.Time, description string) (bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT description, started_at, ended_at
		FROM time_entries
		WHERE operator = 'claude-code'
		  AND cwd = ?
		  AND deleted_at IS NULL
		  AND ended_at IS NOT NULL
		  AND started_at < ?
		  AND ended_at > ?
	`, cwd, endedAt.UTC().Format(timeFormat), startedAt.UTC().Format(timeFormat))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var existingDesc sql.NullString
		var existingStart string
		var existingEnd string
		if err := rows.Scan(&existingDesc, &existingStart, &existingEnd); err != nil {
			return false, err
		}
		existingStartedAt, err := parseTime(existingStart)
		if err != nil {
			return false, err
		}
		existingEndedAt, err := parseTime(existingEnd)
		if err != nil {
			return false, err
		}
		if overlapsMoreThan90Percent(startedAt, endedAt, existingStartedAt, existingEndedAt) && similarDescription(description, existingDesc.String) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func overlapsMoreThan90Percent(aStart, aEnd, bStart, bEnd time.Time) bool {
	start := maxTime(aStart, bStart)
	end := minTime(aEnd, bEnd)
	if !end.After(start) {
		return false
	}
	overlap := end.Sub(start)
	shorter := minDuration(aEnd.Sub(aStart), bEnd.Sub(bStart))
	if shorter <= 0 {
		return false
	}
	return float64(overlap)/float64(shorter) > 0.9
}

func similarDescription(a, b string) bool {
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

func normalizeCompareText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	lastSpace := false
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
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
	var projectColor sql.NullString
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
		&projectColor,
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
	detail.ProjectColor = scanNullString(projectColor)
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
