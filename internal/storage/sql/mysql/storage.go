package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"hubproxy/internal/storage"
	sqlstorage "hubproxy/internal/storage/sql"
)

// MySQLStorage implements the storage.Storage interface for MySQL
type MySQLStorage struct {
	*sqlstorage.BaseStorage
	db *sql.DB
}

// NewStorage creates a new MySQL storage instance
func NewStorage(config storage.Config) (*MySQLStorage, error) {
	connStr := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true",
		config.Username,
		config.Password,
		config.Host,
		config.Port,
		config.Database,
	)

	db, err := sql.Open("mysql", connStr)
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

	return &MySQLStorage{
		BaseStorage: base,
		db:          db,
	}, nil
}

// DefaultConfig returns default MySQL configuration
func DefaultConfig() storage.Config {
	return storage.Config{
		Host:     "localhost",
		Port:     3306,
		Database: "lacrosse",
		Username: "lacrosse",
		Password: "lacrosse",
	}
}

// CreateSchema creates the events table if it doesn't exist
func (s *MySQLStorage) CreateSchema(ctx context.Context) error {
	dialect := NewDialect()
	query := dialect.CreateTableSQL("events")

	if _, err := s.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("creating schema: %w", err)
	}

	return nil
}

// Close closes the database connection
func (s *MySQLStorage) Close() error {
	return s.db.Close()
}

// GetEventTypeStats gets event type statistics since the given time
func (s *MySQLStorage) GetEventTypeStats(ctx context.Context, since time.Time) (map[string]int64, error) {
	stats, err := s.BaseStorage.GetEventTypeStats(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("getting event type stats: %w", err)
	}
	return stats, nil
}
