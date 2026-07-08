package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CreateRepository inserts a new repository record. An empty
// connectorConfig defaults to "{}".
func (s *Store) CreateRepository(ctx context.Context, name string, connectorType ConnectorType, connectorConfig string) (*Repository, error) {
	if connectorConfig == "" {
		connectorConfig = "{}"
	}
	r := &Repository{
		ID:              newID(),
		Name:            name,
		ConnectorType:   connectorType,
		ConnectorConfig: connectorConfig,
		CreatedAt:       time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO repositories (id, name, connector_type, connector_config, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		r.ID, r.Name, string(r.ConnectorType), r.ConnectorConfig, formatTime(r.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("store: create repository: %w", err)
	}
	return r, nil
}

// GetRepository fetches a repository by ID.
func (s *Store) GetRepository(ctx context.Context, id string) (*Repository, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, connector_type, connector_config, created_at
		 FROM repositories WHERE id = ?`, id)
	r, err := scanRepository(row)
	if err != nil {
		return nil, fmt.Errorf("store: get repository %s: %w", id, err)
	}
	return r, nil
}

// ListRepositories returns all repositories, oldest first.
func (s *Store) ListRepositories(ctx context.Context) ([]*Repository, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, connector_type, connector_config, created_at
		 FROM repositories ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list repositories: %w", err)
	}
	defer rows.Close()

	var repos []*Repository
	for rows.Next() {
		r, err := scanRepository(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list repositories: %w", err)
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list repositories: %w", err)
	}
	return repos, nil
}

// EnsureRepository returns the existing repository record, creating one
// with the given attributes if none exists yet. Local mode has a single
// repository — the directory `gloss .` was run against — so "existing"
// means the first (oldest) record.
func (s *Store) EnsureRepository(ctx context.Context, name string, connectorType ConnectorType, connectorConfig string) (*Repository, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, connector_type, connector_config, created_at
		 FROM repositories ORDER BY created_at, id LIMIT 1`)
	r, err := scanRepository(row)
	switch {
	case err == nil:
		return r, nil
	case errors.Is(err, ErrNotFound):
		return s.CreateRepository(ctx, name, connectorType, connectorConfig)
	default:
		return nil, fmt.Errorf("store: ensure repository: %w", err)
	}
}

// RepositorySessionCount returns the number of sessions in a repository.
func (s *Store) RepositorySessionCount(ctx context.Context, repoID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE repo_id = ?`, repoID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count sessions for repository %s: %w", repoID, err)
	}
	return n, nil
}

// scanner abstracts *sql.Row and *sql.Rows for shared scan helpers.
type scanner interface {
	Scan(dest ...any) error
}

func scanRepository(row scanner) (*Repository, error) {
	var r Repository
	var createdAt string
	err := row.Scan(&r.ID, &r.Name, (*string)(&r.ConnectorType), &r.ConnectorConfig, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if r.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	return &r, nil
}
