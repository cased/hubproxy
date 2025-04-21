package storage

import (
	"context"
	"encoding/json"
	"time"
)

// Event represents a GitHub webhook event
type Event struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Headers      json.RawMessage `json:"headers"`
	Payload      json.RawMessage `json:"payload"`
	CreatedAt    time.Time       `json:"created_at"`
	ForwardedAt  *time.Time      `json:"forwarded_at,omitempty"`
	Error        string          `json:"error,omitempty"`
	Repository   string          `json:"repository,omitempty"`
	Sender       string          `json:"sender,omitempty"`
	ReplayedFrom string          `json:"replayed_from,omitempty"` // Original event ID if this is a replay
	OriginalTime time.Time       `json:"original_time,omitempty"` // Original event time if this is a replay
}

// QueryOptions contains options for querying events
type QueryOptions struct {
	Types            []string  // Event types to filter by
	Repository       string    // Repository to filter by
	Sender           string    // Sender to filter by
	Since            time.Time // Start time for events
	Until            time.Time // End time for events
	Limit            int       // Maximum number of events to return
	Offset           int       // Offset for pagination
	OnlyNonForwarded bool      // Only return events that have not been forwarded (forwarded_at IS NULL)
}

// TypeStat represents event type statistics
type TypeStat struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

// Storage defines the interface for event storage
type Storage interface {
	// StoreEvent stores a webhook event
	StoreEvent(ctx context.Context, event *Event) error

	// MarkForwarded marks an event as forwarded by setting the forwarded_at timestamp
	MarkForwarded(ctx context.Context, id string) error

	// ListEvents lists webhook events based on query options
	ListEvents(ctx context.Context, opts QueryOptions) ([]*Event, int, error)

	// CountEvents returns the total number of events matching the given options
	CountEvents(ctx context.Context, opts QueryOptions) (int, error)

	// GetStats returns event type statistics
	GetStats(ctx context.Context, since time.Time) (map[string]int64, error)

	// GetEvent returns a single event by ID
	GetEvent(ctx context.Context, id string) (*Event, error)

	// CreateSchema creates the database schema
	CreateSchema(ctx context.Context) error

	// Close closes the storage
	Close() error
}
