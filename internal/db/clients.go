package db

import (
	"context"
	"database/sql"

	"github.com/akoskm/hrs/internal/model"
	"github.com/oklog/ulid/v2"
)

type ClientListItem struct {
	model.Client
}

func (s *Store) CreateClient(ctx context.Context, name string) (model.Client, error) {
	now := nowUTC()
	id := ulid.Make().String()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO clients (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, id, name, now.Format(timeFormat), now.Format(timeFormat))
	if err != nil {
		return model.Client{}, err
	}
	return s.ClientByName(ctx, name)
}

func (s *Store) ClientByName(ctx context.Context, name string) (model.Client, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, contact, archived_at, created_at, updated_at
		FROM clients WHERE name = ?
	`, name)
	return scanClient(row)
}

func (s *Store) ListClients(ctx context.Context) ([]model.Client, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, contact, archived_at, created_at, updated_at
		FROM clients
		WHERE archived_at IS NULL
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []model.Client
	for rows.Next() {
		client, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	return clients, rows.Err()
}

type clientScanner interface {
	Scan(dest ...any) error
}

func scanClient(row clientScanner) (model.Client, error) {
	var client model.Client
	var contact sql.NullString
	var archivedAt sql.NullString
	var createdAt string
	var updatedAt string
	if err := row.Scan(&client.ID, &client.Name, &contact, &archivedAt, &createdAt, &updatedAt); err != nil {
		return model.Client{}, err
	}
	client.Contact = scanNullString(contact)
	var err error
	client.ArchivedAt, err = scanNullTime(archivedAt)
	if err != nil {
		return model.Client{}, err
	}
	client.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.Client{}, err
	}
	client.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.Client{}, err
	}
	return client, nil
}
