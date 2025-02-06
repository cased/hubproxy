package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"hubproxy/internal/storage"
	sqlstorage "hubproxy/internal/storage/sql"
)

// PostgresStorage implements the storage.Storage interface for PostgreSQL
type PostgresStorage struct {
	*sqlstorage.BaseStorage
	db *sql.DB
}

// NewStorage creates a new PostgreSQL storage instance
func NewStorage(config storage.Config) (*PostgresStorage, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.Host,
		config.Port,
		config.Username,
		config.Password,
		config.Database,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	// Set reasonable defaults
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	dialect := NewDialect()
	base := sqlstorage.NewBaseStorage(db, dialect, "events")

	return &PostgresStorage{
		BaseStorage: base,
		db:          db,
	}, nil
}

// DefaultConfig returns default PostgreSQL configuration
func DefaultConfig() storage.Config {
	return storage.Config{
		Host:     "localhost",
		Port:     5432,
		Database: "lacrosse",
		Username: "lacrosse",
		Password: "lacrosse",
	}
}

// CreateSchema creates the events table if it doesn't exist
func (s *PostgresStorage) CreateSchema(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS events (
			id SERIAL PRIMARY KEY,
			event_type VARCHAR(255) NOT NULL,
			payload JSONB NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := s.db.ExecContext(ctx, query)
	return err
}

// Close closes the database connection
func (s *PostgresStorage) Close() error {
	return s.db.Close()
}

// GetEventTypeStats gets event type statistics since the given time
func (s *PostgresStorage) GetEventTypeStats(ctx context.Context, since time.Time) (map[string]int64, error) {
	query := `
		SELECT event_type, COUNT(*) as count
		FROM events
		WHERE created_at >= $1
		GROUP BY event_type
	`
	rows, err := s.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int64)
	for rows.Next() {
		var eventType string
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, err
		}
		stats[eventType] = count
	}
	return stats, rows.Err()
}
