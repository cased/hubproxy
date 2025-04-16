package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/xo/dburl"

	"hubproxy/internal/storage"
)

type Storage struct {
	*BaseStorage
	db *sql.DB
}

// New creates a new storage instance from a database URI.
// The URI format follows the dburl package conventions:
//   - SQLite: sqlite:/path/to/file.db or sqlite:file.db
//   - MySQL: mysql://user:pass@host/dbname
//   - PostgreSQL: postgres://user:pass@host/dbname
func New(uri string) (storage.Storage, error) {
	// Parse the URL to validate it
	_, err := dburl.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}

	// Create storage using the unified SQL implementation
	store, err := newStorage(context.Background(), uri)
	if err != nil {
		return nil, fmt.Errorf("creating storage: %w", err)
	}

	// Create schema if needed
	if err := store.CreateSchema(context.Background()); err != nil {
		store.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return store, nil
}

func newStorage(ctx context.Context, dsn string) (storage.Storage, error) {
	// Open database using dburl
	db, err := dburl.Open(dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if pingErr := db.PingContext(ctx); pingErr != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", pingErr)
	}

	// Get the driver name from the URL
	u, err := dburl.Parse(dsn)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("parsing DSN: %w", err)
	}

	// Create dialect based on driver
	var dialect SQLDialect
	switch u.Driver {
	case "sqlite3":
		dialect = &SQLiteDialect{}
	case "postgres":
		dialect = &PostgresDialect{}
	case "mysql":
		dialect = &MySQLDialect{}
	default:
		db.Close()
		return nil, fmt.Errorf("unsupported database driver: %s", u.Driver)
	}

	base := NewBaseStorage(db, dialect, "events")
	return &Storage{
		BaseStorage: base,
		db:          db,
	}, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) CreateSchema(ctx context.Context) error {
	sql := s.dialect.CreateTableSQL(s.tableName)
	_, err := s.db.ExecContext(ctx, sql)
	return err
}

func (s *Storage) StoreEvent(ctx context.Context, event *storage.Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	return s.BaseStorage.StoreEvent(ctx, event)
}

func (s *Storage) GetEvent(ctx context.Context, id string) (*storage.Event, error) {
	query := s.builder.
		Select("id", "type", "payload", "created_at", "error", "repository", "sender").
		From(s.tableName).
		Where("id = ?", id).
		Limit(1)

	var event storage.Event
	var payload []byte
	err := query.RunWith(s.db).QueryRowContext(ctx).Scan(
		&event.ID,
		&event.Type,
		&payload,
		&event.CreatedAt,
		&event.Error,
		&event.Repository,
		&event.Sender,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scanning event: %w", err)
	}

	event.Payload = json.RawMessage(payload)
	return &event, nil
}

func (s *Storage) ListEvents(ctx context.Context, opts storage.QueryOptions) ([]*storage.Event, int, error) {
	query := s.builder.
		Select("id", "type", "payload", "created_at", "error", "repository", "sender").
		From(s.tableName)

	query = s.addQueryConditions(query, opts)

	// Get total count first
	countQuery := s.builder.Select("COUNT(*)").From(s.tableName)
	countQuery = s.addQueryConditions(countQuery, opts)

	var total int
	err := countQuery.RunWith(s.db).QueryRowContext(ctx).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting events: %w", err)
	}

	// Add pagination
	if opts.Limit > 0 {
		query = query.Limit(uint64(opts.Limit))
	}
	if opts.Offset > 0 {
		query = query.Offset(uint64(opts.Offset))
	}

	rows, err := query.RunWith(s.db).QueryContext(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var events []*storage.Event
	for rows.Next() {
		var event storage.Event
		var payload []byte
		err := rows.Scan(
			&event.ID,
			&event.Type,
			&payload,
			&event.CreatedAt,
			&event.Error,
			&event.Repository,
			&event.Sender,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning event: %w", err)
		}
		event.Payload = json.RawMessage(payload)
		events = append(events, &event)
	}

	return events, total, rows.Err()
}

func (s *Storage) CountEvents(ctx context.Context, opts storage.QueryOptions) (int, error) {
	query := s.builder.Select("COUNT(*)").From(s.tableName)
	query = s.addQueryConditions(query, opts)

	var count int
	err := query.RunWith(s.db).QueryRowContext(ctx).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting events: %w", err)
	}

	return count, nil
}

func (s *Storage) UpdateEventStatus(ctx context.Context, id string, status string, err error) error {
	var errStr string
	if err != nil {
		errStr = err.Error()
	}

	query := s.builder.
		Update(s.tableName).
		Set("error", errStr).
		Where("id = ?", id)

	result, err := query.RunWith(s.db).ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("updating event status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("event not found")
	}

	return nil
}

func (s *Storage) MarkForwarded(ctx context.Context, id string) error {
	query := s.builder.
		Update(s.tableName).
		Set("forwarded_at", time.Now()).
		Where("id = ?", id)

	result, err := query.RunWith(s.db).ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("marking event as forwarded: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("event not found")
	}
	return nil
}

func (s *Storage) GetStats(ctx context.Context, since time.Time) (map[string]int64, error) {
	query := s.builder.
		Select("type", "COUNT(*) as count").
		From(s.tableName).
		GroupBy("type")

	if !since.IsZero() {
		query = query.Where("created_at >= ?", since)
	}

	rows, err := query.RunWith(s.db).QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int64)
	for rows.Next() {
		var (
			eventType string
			count     int64
		)
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("scanning stats: %w", err)
		}
		stats[eventType] = count
	}

	return stats, rows.Err()
}
