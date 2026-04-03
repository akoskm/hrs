package db

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/akoskm/hrs/internal/model"
	"github.com/oklog/ulid/v2"
)

type ProjectPathMatch struct {
	ProjectID string
	Path      string
}

func (s *Store) AddProjectPath(ctx context.Context, projectIdent, path string) (model.ProjectPath, error) {
	project, err := s.ProjectByCodeOrName(ctx, projectIdent)
	if err != nil {
		return model.ProjectPath{}, err
	}
	normalized := filepath.Clean(path)
	id := ulid.Make().String()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO project_paths (id, project_id, path)
		VALUES (?, ?, ?)
	`, id, project.ID, normalized)
	if err != nil {
		return model.ProjectPath{}, err
	}
	return model.ProjectPath{ID: id, ProjectID: project.ID, Path: normalized}, nil
}

func (s *Store) DetectProjectIDByPath(ctx context.Context, cwd string) (*string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT project_id, path
		FROM project_paths
		ORDER BY length(path) DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	normalizedCWD := filepath.Clean(cwd)
	for rows.Next() {
		var match ProjectPathMatch
		if err := rows.Scan(&match.ProjectID, &match.Path); err != nil {
			return nil, err
		}
		if pathContains(match.Path, normalizedCWD) {
			return &match.ProjectID, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

func pathContains(base, current string) bool {
	base = filepath.Clean(base)
	current = filepath.Clean(current)
	if current == base {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(current, base+sep)
}
