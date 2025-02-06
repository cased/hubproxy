package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"hubproxy/internal/storage"
	"hubproxy/internal/storage/sql/sqldb"
)

type Storage struct {
	db      *pgxpool.Pool
	queries *sqldb.Queries
}

func NewStorage(ctx context.Context, dsn string) (*Storage, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing connection string: %w", err)
	}

	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	if err := db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &Storage{
		db:      db,
		queries: sqldb.New(db),
	}, nil
}

func (s *Storage) Close() {
	s.db.Close()
}

func (s *Storage) CreateEvent(ctx context.Context, event *storage.Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}

	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	dbEvent, err := s.queries.CreateEvent(ctx, sqldb.CreateEventParams{
		ID:         event.ID,
		Type:       event.Type,
		Payload:    payload,
		CreatedAt:  event.CreatedAt,
		Status:     event.Status,
		Error:      event.Error,
		Repository: event.Repository,
		Sender:     event.Sender,
	})
	if err != nil {
		return fmt.Errorf("creating event: %w", err)
	}

	event.ID = dbEvent.ID
	return nil
}

func (s *Storage) GetEvent(ctx context.Context, id string) (*storage.Event, error) {
	dbEvent, err := s.queries.GetEvent(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("event not found")
		}
		return nil, fmt.Errorf("getting event: %w", err)
	}

	return convertDBEvent(dbEvent), nil
}

func (s *Storage) ListEvents(ctx context.Context, opts storage.QueryOptions) ([]*storage.Event, error) {
	params := sqldb.ListEventsParams{
		Column1: opts.Types,
		Column2: opts.Repository,
		Column5: opts.Status,
		Column6: opts.Sender,
	}

	// Ensure values are within int32 bounds
	limit := opts.Limit
	if limit < math.MinInt32 {
		limit = math.MinInt32
	} else if limit > math.MaxInt32 {
		limit = math.MaxInt32
	}
	offset := opts.Offset
	if offset < math.MinInt32 {
		offset = math.MinInt32
	} else if offset > math.MaxInt32 {
		offset = math.MaxInt32
	}
	// Safe to convert to int32 since values are guaranteed to be within bounds
	//nolint:gosec // Values are guaranteed to be within int32 bounds
	params.Limit = int32(limit)
	params.Offset = int32(offset) //nolint:gosec // Values are guaranteed to be within int32 bounds

	if !opts.Since.IsZero() {
		params.Column3 = pgtype.Timestamp{Time: opts.Since, Valid: true}
	}
	if !opts.Until.IsZero() {
		params.Column4 = pgtype.Timestamp{Time: opts.Until, Valid: true}
	}

	dbEvents, err := s.queries.ListEvents(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	events := make([]*storage.Event, len(dbEvents))
	for i, dbEvent := range dbEvents {
		events[i] = convertDBEvent(dbEvent)
	}
	return events, nil
}

func (s *Storage) CountEvents(ctx context.Context, opts storage.QueryOptions) (int, error) {
	params := sqldb.CountEventsParams{
		Column1: opts.Types,
		Column2: opts.Repository,
		Column5: opts.Status,
		Column6: opts.Sender,
	}

	if !opts.Since.IsZero() {
		params.Column3 = pgtype.Timestamp{Time: opts.Since, Valid: true}
	}
	if !opts.Until.IsZero() {
		params.Column4 = pgtype.Timestamp{Time: opts.Until, Valid: true}
	}

	count, err := s.queries.CountEvents(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("counting events: %w", err)
	}

	return int(count), nil
}

func (s *Storage) UpdateEventStatus(ctx context.Context, id string, status string, err error) error {
	var errStr string
	if err != nil {
		errStr = err.Error()
	}

	_, dbErr := s.queries.UpdateEventStatus(ctx, sqldb.UpdateEventStatusParams{
		ID:     id,
		Status: status,
		Error:  errStr,
	})
	if dbErr != nil {
		if dbErr == pgx.ErrNoRows {
			return fmt.Errorf("event not found")
		}
		return fmt.Errorf("updating event status: %w", dbErr)
	}

	return nil
}

func (s *Storage) GetEventTypeStats(ctx context.Context, since, until time.Time) ([]storage.TypeStat, error) {
	params := sqldb.GetEventTypeStatsParams{}
	if !since.IsZero() {
		params.Column1 = pgtype.Timestamp{Time: since, Valid: true}
	}
	if !until.IsZero() {
		params.Column2 = pgtype.Timestamp{Time: until, Valid: true}
	}

	stats, err := s.queries.GetEventTypeStats(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("getting event type stats: %w", err)
	}

	result := make([]storage.TypeStat, len(stats))
	for i, stat := range stats {
		result[i] = storage.TypeStat{
			Type:  stat.Type,
			Count: stat.Count,
		}
	}
	return result, nil
}

func convertDBEvent(e sqldb.Event) *storage.Event {
	var payload json.RawMessage
	if len(e.Payload) > 0 {
		payload = json.RawMessage(e.Payload)
	}
	return &storage.Event{
		ID:         e.ID,
		Type:       e.Type,
		Payload:    payload,
		CreatedAt:  e.CreatedAt,
		Status:     e.Status,
		Error:      e.Error,
		Repository: e.Repository,
		Sender:     e.Sender,
	}
}
