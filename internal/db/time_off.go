package db

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/akoskm/hrs/internal/model"
	"github.com/oklog/ulid/v2"
)

var defaultTimeOffTypeNames = []string{"Holiday", "Sick Leave", "Vacation"}

func (s *Store) EnsureProjectDefaultTimeOffTypes(ctx context.Context, projectID string) error {
	for _, name := range defaultTimeOffTypeNames {
		if _, err := s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO time_off_types (id, project_id, name, is_system, created_at, updated_at)
			VALUES (?, ?, ?, 1, ?, ?)
		`, ulid.Make().String(), projectID, name, nowUTC().Format(timeFormat), nowUTC().Format(timeFormat)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListTimeOffTypesByProject(ctx context.Context, projectID string) ([]model.TimeOffType, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, name, is_system, archived_at, created_at, updated_at
		FROM time_off_types
		WHERE project_id = ? AND archived_at IS NULL
		ORDER BY name
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []model.TimeOffType
	for rows.Next() {
		item, err := scanTimeOffType(rows)
		if err != nil {
			return nil, err
		}
		types = append(types, item)
	}
	return types, rows.Err()
}

type timeOffTypeScanner interface {
	Scan(dest ...any) error
}

func scanTimeOffType(row timeOffTypeScanner) (model.TimeOffType, error) {
	var item model.TimeOffType
	var archivedAt sql.NullString
	var createdAt string
	var updatedAt string
	var isSystem int
	if err := row.Scan(&item.ID, &item.ProjectID, &item.Name, &isSystem, &archivedAt, &createdAt, &updatedAt); err != nil {
		return model.TimeOffType{}, err
	}
	item.IsSystem = isSystem == 1
	var err error
	item.ArchivedAt, err = scanNullTime(archivedAt)
	if err != nil {
		return model.TimeOffType{}, err
	}
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.TimeOffType{}, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.TimeOffType{}, err
	}
	return item, nil
}

func normalizeTimeOffTypeName(name string) string {
	return strings.TrimSpace(name)
}

func (s *Store) CreateTimeOffType(ctx context.Context, projectID, name string) (model.TimeOffType, error) {
	trimmed := normalizeTimeOffTypeName(name)
	id := ulid.Make().String()
	now := nowUTC().Format(timeFormat)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO time_off_types (id, project_id, name, is_system, created_at, updated_at)
		VALUES (?, ?, ?, 0, ?, ?)
	`, id, projectID, trimmed, now, now); err != nil {
		return model.TimeOffType{}, err
	}
	return s.TimeOffTypeByID(ctx, id)
}

func (s *Store) TimeOffTypeByID(ctx context.Context, id string) (model.TimeOffType, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, is_system, archived_at, created_at, updated_at
		FROM time_off_types
		WHERE id = ?
	`, id)
	return scanTimeOffType(row)
}

func (s *Store) UpsertTimeOffDay(ctx context.Context, projectID, timeOffTypeID, day string) (model.TimeOffDay, error) {
	now := nowUTC().Format(timeFormat)
	row := s.db.QueryRowContext(ctx, `SELECT id FROM time_off_days WHERE project_id = ? AND day = ?`, projectID, day)
	var id string
	err := row.Scan(&id)
	if err != nil {
		if err != sql.ErrNoRows {
			return model.TimeOffDay{}, err
		}
		id = ulid.Make().String()
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO time_off_days (id, project_id, time_off_type_id, day, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, id, projectID, timeOffTypeID, day, now, now); err != nil {
			return model.TimeOffDay{}, err
		}
		return s.TimeOffDayByID(ctx, id)
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE time_off_days
		SET time_off_type_id = ?, updated_at = ?
		WHERE id = ?
	`, timeOffTypeID, now, id); err != nil {
		return model.TimeOffDay{}, err
	}
	return s.TimeOffDayByID(ctx, id)
}

func (s *Store) DeleteTimeOffDay(ctx context.Context, projectID, day string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM time_off_days WHERE project_id = ? AND day = ?`, projectID, day)
	return err
}

func (s *Store) ListTimeOffDaysInRange(ctx context.Context, startDay, endDay string) ([]model.TimeOffDayDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			tod.id, tod.project_id, tod.time_off_type_id, tod.day, tod.created_at, tod.updated_at,
			p.name, p.code, tot.name
		FROM time_off_days tod
		JOIN projects p ON p.id = tod.project_id
		JOIN time_off_types tot ON tot.id = tod.time_off_type_id
		WHERE tod.day >= ? AND tod.day <= ?
		ORDER BY tod.day, p.name, tot.name
	`, startDay, endDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []model.TimeOffDayDetail
	for rows.Next() {
		record, err := scanTimeOffDayDetail(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) TimeOffDayByID(ctx context.Context, id string) (model.TimeOffDay, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, time_off_type_id, day, created_at, updated_at
		FROM time_off_days
		WHERE id = ?
	`, id)
	return scanTimeOffDay(row)
}

type timeOffDayScanner interface {
	Scan(dest ...any) error
}

func scanTimeOffDay(row timeOffDayScanner) (model.TimeOffDay, error) {
	var item model.TimeOffDay
	var createdAt string
	var updatedAt string
	if err := row.Scan(&item.ID, &item.ProjectID, &item.TimeOffTypeID, &item.Day, &createdAt, &updatedAt); err != nil {
		return model.TimeOffDay{}, err
	}
	var err error
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.TimeOffDay{}, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.TimeOffDay{}, err
	}
	return item, nil
}

func scanTimeOffDayDetail(row timeOffDayScanner) (model.TimeOffDayDetail, error) {
	var item model.TimeOffDayDetail
	var projectCode sql.NullString
	var createdAt string
	var updatedAt string
	if err := row.Scan(
		&item.ID,
		&item.ProjectID,
		&item.TimeOffTypeID,
		&item.Day,
		&createdAt,
		&updatedAt,
		&item.ProjectName,
		&projectCode,
		&item.TimeOffType,
	); err != nil {
		return model.TimeOffDayDetail{}, err
	}
	item.ProjectCode = scanNullString(projectCode)
	var err error
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model.TimeOffDayDetail{}, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return model.TimeOffDayDetail{}, err
	}
	return item, nil
}
