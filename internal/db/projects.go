package db

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/akoskm/hrs/internal/model"
	"github.com/oklog/ulid/v2"
)

var ErrProjectNotFound = errors.New("project not found")

type ProjectCreateInput struct {
	ClientName string
	Name       string
	Code       string
	HourlyRate int
	Currency   model.Currency
	Color      string
}

type ProjectListItem struct {
	model.Project
	ClientName *string
}

func (s *Store) UpdateProjectBillableDefault(ctx context.Context, ident string, billable bool) (model.Project, error) {
	project, err := s.ProjectByCodeOrName(ctx, ident)
	if err != nil {
		return model.Project{}, err
	}
	return s.UpdateProjectBillableDefaultByID(ctx, project.ID, billable)
}

func (s *Store) UpdateProjectBillableDefaultByID(ctx context.Context, id string, billable bool) (model.Project, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE projects
		SET billable_default = ?, updated_at = ?
		WHERE id = ?
	`, boolToInt(billable), nowUTC().Format(timeFormat), id)
	if err != nil {
		return model.Project{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return model.Project{}, err
	}
	if rows == 0 {
		return model.Project{}, ErrProjectNotFound
	}
	return s.ProjectByID(ctx, id)
}

func (s *Store) UpdateProjectColorByID(ctx context.Context, id, color string) (model.Project, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE projects
		SET color = ?, updated_at = ?
		WHERE id = ?
	`, color, nowUTC().Format(timeFormat), id)
	if err != nil {
		return model.Project{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return model.Project{}, err
	}
	if rows == 0 {
		return model.Project{}, ErrProjectNotFound
	}
	return s.ProjectByID(ctx, id)
}

func (s *Store) ArchiveProjectByID(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE projects
		SET archived_at = ?, updated_at = ?
		WHERE id = ? AND archived_at IS NULL
	`, nowUTC().Format(timeFormat), nowUTC().Format(timeFormat), id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrProjectNotFound
	}
	return nil
}

func (s *Store) CreateProject(ctx context.Context, input ProjectCreateInput) (model.Project, error) {
	now := nowUTC()
	id := ulid.Make().String()
	if strings.TrimSpace(input.Code) == "" {
		input.Code = slugProjectCode(input.Name)
	}
	var clientID any
	if strings.TrimSpace(input.ClientName) != "" {
		client, err := s.ClientByName(ctx, strings.TrimSpace(input.ClientName))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.Project{}, ErrProjectNotFound
			}
			return model.Project{}, err
		}
		clientID = client.ID
	}
	var code any
	if input.Code != "" {
		code = input.Code
	}
	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = defaultProjectColor(id)
	}
	currency := input.Currency
	if currency == "" {
		currency = model.CurrencyEUR
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (id, client_id, name, code, hourly_rate, currency, billable_default, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
	`, id, clientID, input.Name, code, input.HourlyRate, currency, color, now.Format(timeFormat), now.Format(timeFormat))
	if err != nil {
		return model.Project{}, err
	}
	return s.ProjectByID(ctx, id)
}

func (s *Store) EnsureProject(ctx context.Context, name, code string) (model.Project, error) {
	project, err := s.ProjectByCode(ctx, code)
	if err == nil {
		return project, nil
	}
	if err != sql.ErrNoRows {
		return model.Project{}, err
	}

	now := nowUTC()
	id := ulid.Make().String()
	color := defaultProjectColor(id)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO projects (id, name, code, hourly_rate, currency, billable_default, color, created_at, updated_at)
		VALUES (?, ?, ?, 0, 'EUR', 1, ?, ?, ?)
	`, id, name, code, color, now.Format(timeFormat), now.Format(timeFormat))
	if err != nil {
		return model.Project{}, err
	}
	return s.ProjectByID(ctx, id)
}

func (s *Store) ListProjects(ctx context.Context) ([]model.Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, client_id, name, code, hourly_rate, currency, billable_default, color, archived_at, created_at, updated_at
		FROM projects
		WHERE archived_at IS NULL
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *Store) ListProjectsWithClient(ctx context.Context) ([]ProjectListItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.client_id, p.name, p.code, p.hourly_rate, p.currency, p.billable_default, p.color, p.archived_at, p.created_at, p.updated_at, c.name
		FROM projects p
		LEFT JOIN clients c ON c.id = p.client_id
		WHERE p.archived_at IS NULL
		ORDER BY p.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ProjectListItem
	for rows.Next() {
		item, err := scanProjectListItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ProjectByCode(ctx context.Context, code string) (model.Project, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, client_id, name, code, hourly_rate, currency, billable_default, color, archived_at, created_at, updated_at
		FROM projects WHERE code = ?
	`, code)
	return scanProject(row)
}

func (s *Store) ProjectByName(ctx context.Context, name string) (model.Project, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, client_id, name, code, hourly_rate, currency, billable_default, color, archived_at, created_at, updated_at
		FROM projects WHERE name = ?
	`, name)
	return scanProject(row)
}

func (s *Store) ProjectByCodeOrName(ctx context.Context, ident string) (model.Project, error) {
	project, err := s.ProjectByCode(ctx, ident)
	if err == nil {
		return project, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return model.Project{}, err
	}
	project, err = s.ProjectByName(ctx, ident)
	if err == nil {
		return project, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return model.Project{}, ErrProjectNotFound
	}
	return model.Project{}, err
}

func (s *Store) ProjectByID(ctx context.Context, id string) (model.Project, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, client_id, name, code, hourly_rate, currency, billable_default, color, archived_at, created_at, updated_at
		FROM projects WHERE id = ?
	`, id)
	return scanProject(row)
}

type projectScanner interface {
	Scan(dest ...any) error
}

const timeFormat = time.RFC3339Nano

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

var defaultProjectColors = []string{
	"#ff6b6b",
	"#f59f00",
	"#ffd43b",
	"#51cf66",
	"#20c997",
	"#22b8cf",
	"#4dabf7",
	"#748ffc",
	"#9775fa",
	"#e599f7",
	"#f06595",
	"#ff8787",
}

func slugProjectCode(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonSlugChars.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "project"
	}
	return slug
}

func defaultProjectColor(seed string) string {
	if len(defaultProjectColors) == 0 {
		return "#748ffc"
	}
	sum := 0
	for i := 0; i < len(seed); i++ {
		sum += int(seed[i])
	}
	return defaultProjectColors[sum%len(defaultProjectColors)]
}

func DefaultProjectColors() []string {
	colors := make([]string, len(defaultProjectColors))
	copy(colors, defaultProjectColors)
	return colors
}

func scanProject(row projectScanner) (model.Project, error) {
	var project model.Project
	var clientID sql.NullString
	var code sql.NullString
	var color sql.NullString
	var archivedAt sql.NullString
	var createdAt string
	var updatedAt string
	var billableDefault int
	if err := row.Scan(
		&project.ID,
		&clientID,
		&project.Name,
		&code,
		&project.HourlyRate,
		&project.Currency,
		&billableDefault,
		&color,
		&archivedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.Project{}, err
	}
	project.ClientID = scanNullString(clientID)
	project.Code = scanNullString(code)
	project.Color = scanNullString(color)
	project.BillableDefault = billableDefault == 1
	var err error
	project.ArchivedAt, err = scanNullTime(archivedAt)
	if err != nil {
		return model.Project{}, err
	}
	project.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.Project{}, err
	}
	project.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.Project{}, err
	}
	return project, nil
}

func scanProjectListItem(row projectScanner) (ProjectListItem, error) {
	var item ProjectListItem
	var clientID sql.NullString
	var code sql.NullString
	var color sql.NullString
	var archivedAt sql.NullString
	var createdAt string
	var updatedAt string
	var clientName sql.NullString
	var billableDefault int
	if err := row.Scan(
		&item.ID,
		&clientID,
		&item.Name,
		&code,
		&item.HourlyRate,
		&item.Currency,
		&billableDefault,
		&color,
		&archivedAt,
		&createdAt,
		&updatedAt,
		&clientName,
	); err != nil {
		return ProjectListItem{}, err
	}
	item.ClientID = scanNullString(clientID)
	item.Code = scanNullString(code)
	item.Color = scanNullString(color)
	item.ClientName = scanNullString(clientName)
	item.BillableDefault = billableDefault == 1
	var err error
	item.ArchivedAt, err = scanNullTime(archivedAt)
	if err != nil {
		return ProjectListItem{}, err
	}
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return ProjectListItem{}, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return ProjectListItem{}, err
	}
	return item, nil
}
