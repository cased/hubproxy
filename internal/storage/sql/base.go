package sql

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"hubproxy/internal/storage"

	sq "github.com/Masterminds/squirrel"
)

// BaseStorage provides common SQL storage implementations
type BaseStorage struct {
	db        *sql.DB
	dialect   SQLDialect
	tableName string
	// Use squirrel's placeholder format based on dialect
	builder sq.StatementBuilderType
}

// NewBaseStorage creates a new BaseStorage
func NewBaseStorage(db *sql.DB, dialect SQLDialect, tableName string) *BaseStorage {
	// Choose placeholder format based on dialect
	var builder sq.StatementBuilderType
	if dialect.PlaceholderFormat() == "?" {
		builder = sq.StatementBuilder.PlaceholderFormat(sq.Question)
	} else {
		builder = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	}

	return &BaseStorage{
		db:        db,
		dialect:   dialect,
		tableName: tableName,
		builder:   builder,
	}
}

// StoreEvent stores a webhook event in the database
func (s *BaseStorage) StoreEvent(ctx context.Context, event *storage.Event) error {
	// Check if event exists
	exists := false
	err := s.builder.Select("1").
		From(s.tableName).
		Where("id = ?", event.ID).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("checking event existence: %w", err)
	}

	if exists {
		// Update existing event
		query := s.builder.Update(s.tableName).
			Set("type", event.Type).
			Set("payload", event.Payload).
			Set("created_at", event.CreatedAt).
			Set("status", event.Status).
			Set("error", event.Error).
			Set("repository", event.Repository).
			Set("sender", event.Sender).
			Where("id = ?", event.ID)

		_, err = query.RunWith(s.db).ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("updating event: %w", err)
		}
		return nil
	}

	// Insert new event
	query := s.builder.Insert(s.tableName).
		Columns("id", "type", "payload", "created_at", "status", "error", "repository", "sender").
		Values(
			event.ID,
			event.Type,
			event.Payload,
			event.CreatedAt,
			event.Status,
			event.Error,
			event.Repository,
			event.Sender,
		)

	_, err = query.RunWith(s.db).ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("inserting event: %w", err)
	}
	return nil
}

// ListEvents lists webhook events based on query options
func (s *BaseStorage) ListEvents(ctx context.Context, opts storage.QueryOptions) ([]*storage.Event, int, error) {
	// Build base query
	query := s.builder.Select(
		"id", "type", "payload", "created_at", "status", "error", "repository", "sender",
	).From(s.tableName)

	// Add conditions
	query = s.addQueryConditions(query, opts)

	// Add order and limit
	query = query.OrderBy("created_at DESC")
	if opts.Limit > 0 {
		// Ensure values are within uint64 bounds
		limit := opts.Limit
		if limit < 0 {
			limit = 0
		} else if limit > math.MaxInt {
			limit = math.MaxInt
		}
		offset := opts.Offset
		if offset < 0 {
			offset = 0
		} else if offset > math.MaxInt {
			offset = math.MaxInt
		}
		// Safe to convert to uint64 since values are guaranteed to be non-negative and <= MaxInt
		//nolint:gosec // Values are guaranteed to be non-negative and <= MaxInt
		query = query.Limit(uint64(limit)).Offset(uint64(offset))
	}

	// Execute query
	rows, err := query.RunWith(s.db).QueryContext(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("executing query: %w", err)
	}
	defer rows.Close()

	// Scan results
	var events []*storage.Event
	for rows.Next() {
		var event storage.Event
		scanErr := rows.Scan(
			&event.ID,
			&event.Type,
			&event.Payload,
			&event.CreatedAt,
			&event.Status,
			&event.Error,
			&event.Repository,
			&event.Sender,
		)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scanning row: %w", scanErr)
		}
		events = append(events, &event)
	}

	// Get total count
	total, err := s.CountEvents(ctx, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("getting total count: %w", err)
	}

	return events, total, nil
}

// CountEvents returns the total number of events matching the given options
func (s *BaseStorage) CountEvents(ctx context.Context, opts storage.QueryOptions) (int, error) {
	query := s.builder.Select("COUNT(*)").From(s.tableName)
	query = s.addQueryConditions(query, opts)

	var count int
	err := query.RunWith(s.db).QueryRowContext(ctx).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting events: %w", err)
	}

	return count, nil
}

// GetStats returns event type statistics
func (s *BaseStorage) GetStats(ctx context.Context, since time.Time) (map[string]int64, error) {
	query := s.builder.
		Select("type", "COUNT(*) as count").
		From(s.tableName).
		GroupBy("type")

	if !since.IsZero() {
		query = query.Where(sq.GtOrEq{"created_at": since})
	}

	rows, err := query.RunWith(s.db).QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("executing query: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int64)
	for rows.Next() {
		var (
			eventType string
			count     int64
		)
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		stats[eventType] = count
	}

	return stats, nil
}

// GetEvent returns a single event by ID
func (s *BaseStorage) GetEvent(ctx context.Context, id string) (*storage.Event, error) {
	query := s.builder.
		Select("id", "type", "payload", "created_at", "status", "error", "repository", "sender").
		From(s.tableName).
		Where(sq.Eq{"id": id}).
		Limit(1)

	rows, err := query.RunWith(s.db).QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("executing query: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}

	event := &storage.Event{}
	scanErr := rows.Scan(
		&event.ID,
		&event.Type,
		&event.Payload,
		&event.CreatedAt,
		&event.Status,
		&event.Error,
		&event.Repository,
		&event.Sender,
	)
	if scanErr != nil {
		return nil, fmt.Errorf("scanning row: %w", scanErr)
	}

	return event, nil
}

// addQueryConditions adds WHERE conditions based on query options
func (s *BaseStorage) addQueryConditions(query sq.SelectBuilder, opts storage.QueryOptions) sq.SelectBuilder {
	if len(opts.Types) > 0 {
		query = query.Where(sq.Eq{"type": opts.Types})
	}
	if !opts.Since.IsZero() {
		query = query.Where(sq.GtOrEq{"created_at": opts.Since})
	}
	if !opts.Until.IsZero() {
		query = query.Where(sq.LtOrEq{"created_at": opts.Until})
	}
	if opts.Status != "" {
		query = query.Where(sq.Eq{"status": opts.Status})
	}
	if opts.Repository != "" {
		query = query.Where(sq.Eq{"repository": opts.Repository})
	}
	if opts.Sender != "" {
		query = query.Where(sq.Eq{"sender": opts.Sender})
	}
	return query
}
