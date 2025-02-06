package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"hubproxy/internal/storage"
	sqlstorage "hubproxy/internal/storage/sql"
)

type SQLiteStorage struct {
	*sqlstorage.BaseStorage
	db *sql.DB
}

func NewStorage(path string) (storage.Storage, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable foreign keys and WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;"); err != nil {
		return nil, fmt.Errorf("configuring database: %w", err)
	}

	dialect := NewDialect()
	base := sqlstorage.NewBaseStorage(db, dialect, "events")

	return &SQLiteStorage{
		BaseStorage: base,
		db:          db,
	}, nil
}

func (s *SQLiteStorage) CreateSchema(ctx context.Context) error {
	dialect := NewDialect()
	query := dialect.CreateTableSQL("events")

	_, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("creating schema: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) StoreEvent(ctx context.Context, event *storage.Event) error {
	query := `
		INSERT INTO events (id, type, payload, created_at, status, error, repository, sender)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			payload = excluded.payload,
			created_at = excluded.created_at,
			status = excluded.status,
			error = excluded.error,
			repository = excluded.repository,
			sender = excluded.sender
	`

	_, err := s.db.ExecContext(ctx, query,
		event.ID,
		event.Type,
		event.Payload,
		event.CreatedAt,
		event.Status,
		event.Error,
		event.Repository,
		event.Sender,
	)
	return err
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}
