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
	ForwardedAt  *time.Time      `json:"forwarded_at,omitempty"` // Time when the event was forwarded to the target
	Error        string          `json:"error,omitempty"`
	Repository   string          `json:"repository,omitempty"`
	Sender       string          `json:"sender,omitempty"`
	ReplayedFrom string          `json:"replayed_from,omitempty"` // Original event ID if this is a replay
	OriginalTime time.Time       `json:"original_time,omitempty"` // Original event time if this is a replay
}

// QueryOptions contains options for querying events
type QueryOptions struct {
	Types      []string  // Event types to filter by
	Repository string    // Repository to filter by
	Sender     string    // Sender to filter by
	Since      time.Time // Start time for events
	Until      time.Time // End time for events
	Limit      int       // Maximum number of events to return
	Offset     int       // Offset for pagination
}

// TypeStat represents event type statistics
type TypeStat struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

// Storage defines the interface for event storage
type Storage interface {
	StoreEvent(ctx context.Context, event *Event) error
	GetEvent(ctx context.Context, id string) (*Event, error)
	ListEvents(ctx context.Context, opts QueryOptions) ([]*Event, int, error)
	CountEvents(ctx context.Context, opts QueryOptions) (int, error)
	GetStats(ctx context.Context, since time.Time) (map[string]int64, error)
	MarkForwarded(ctx context.Context, id string) error
	CreateSchema(ctx context.Context) error
	Close() error
}
