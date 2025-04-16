package graphql

import (
	"fmt"
	"time"

	"hubproxy/internal/storage"

	"github.com/google/uuid"
	"github.com/graphql-go/graphql"
)

// resolveEvents handles the events query
func (s *Schema) resolveEvents(p graphql.ResolveParams) (interface{}, error) {
	// Parse query parameters
	opts := storage.QueryOptions{
		Limit:  50, // Default limit
		Offset: 0,  // Default offset
	}

	// Parse type filter
	if t, ok := p.Args["type"].(string); ok && t != "" {
		opts.Types = []string{t} // Single type for now
	}

	// Parse other filters
	if repo, ok := p.Args["repository"].(string); ok && repo != "" {
		opts.Repository = repo
	}

	if sender, ok := p.Args["sender"].(string); ok && sender != "" {
		opts.Sender = sender
	}

	if status, ok := p.Args["status"].(string); ok && status != "" {
		opts.Status = status
	}

	// Parse since/until
	if since, ok := p.Args["since"].(time.Time); ok {
		opts.Since = since
	}

	if until, ok := p.Args["until"].(time.Time); ok {
		opts.Until = until
	}

	// Parse limit/offset
	if limit, ok := p.Args["limit"].(int); ok && limit > 0 {
		opts.Limit = limit
	}

	if offset, ok := p.Args["offset"].(int); ok && offset >= 0 {
		opts.Offset = offset
	}

	// Get events
	events, total, err := s.store.ListEvents(p.Context, opts)
	if err != nil {
		s.logger.Error("Error listing events", "error", err)
		return nil, err
	}

	return map[string]interface{}{
		"events": events,
		"total":  total,
	}, nil
}

// resolveEvent handles the event query
func (s *Schema) resolveEvent(p graphql.ResolveParams) (interface{}, error) {
	id, ok := p.Args["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("invalid event ID")
	}

	event, err := s.store.GetEvent(p.Context, id)
	if err != nil {
		s.logger.Error("Error getting event", "error", err)
		return nil, err
	}

	if event == nil {
		return nil, fmt.Errorf("event not found")
	}

	return event, nil
}

// resolveStats handles the stats query
func (s *Schema) resolveStats(p graphql.ResolveParams) (interface{}, error) {
	var since time.Time
	if sinceArg, ok := p.Args["since"].(time.Time); ok {
		since = sinceArg
	}

	statsMap, err := s.store.GetStats(p.Context, since)
	if err != nil {
		s.logger.Error("Error getting stats", "error", err)
		return nil, err
	}

	// Convert map to array of stats
	stats := make([]map[string]interface{}, 0, len(statsMap))
	for eventType, count := range statsMap {
		stats = append(stats, map[string]interface{}{
			"type":  eventType,
			"count": count,
		})
	}

	return stats, nil
}

// resolveReplayEvent handles the replayEvent mutation
func (s *Schema) resolveReplayEvent(p graphql.ResolveParams) (interface{}, error) {
	id, ok := p.Args["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("invalid event ID")
	}

	// Get event from storage
	event, err := s.store.GetEvent(p.Context, id)
	if err != nil {
		s.logger.Error("Error getting event", "error", err)
		return nil, err
	}

	if event == nil {
		return nil, fmt.Errorf("event not found")
	}

	// Create new event with same payload but new ID and timestamp
	replayEvent := &storage.Event{
		ID:           fmt.Sprintf("%s-replay-%s", event.ID, uuid.New().String()), // Format: original-id-replay-uuid
		Type:         event.Type,
		Payload:      event.Payload,
		Headers:      event.Headers,
		CreatedAt:    time.Now(),
		Status:       "replayed",
		Repository:   event.Repository,
		Sender:       event.Sender,
		ReplayedFrom: event.ID,
		OriginalTime: event.CreatedAt,
	}

	// Store the replayed event
	if err := s.store.StoreEvent(p.Context, replayEvent); err != nil {
		s.logger.Error("Error storing replayed event", "error", err)
		return nil, err
	}

	return map[string]interface{}{
		"replayedCount": 1,
		"events":        []*storage.Event{replayEvent},
	}, nil
}

// resolveReplayRange handles the replayRange mutation
func (s *Schema) resolveReplayRange(p graphql.ResolveParams) (interface{}, error) {
	// Parse query parameters for time range
	opts := storage.QueryOptions{
		Limit:  100, // Default limit for replay
		Offset: 0,
	}

	// Parse limit if provided
	if limit, ok := p.Args["limit"].(int); ok && limit > 0 {
		opts.Limit = limit
	}

	// Parse since/until (both required for range replay)
	since, ok := p.Args["since"].(time.Time)
	if !ok {
		return nil, fmt.Errorf("missing since parameter")
	}
	opts.Since = since

	until, ok := p.Args["until"].(time.Time)
	if !ok {
		return nil, fmt.Errorf("missing until parameter")
	}
	opts.Until = until

	// Optional filters
	if t, ok := p.Args["type"].(string); ok && t != "" {
		opts.Types = []string{t}
	}

	if repo, ok := p.Args["repository"].(string); ok && repo != "" {
		opts.Repository = repo
	}

	if sender, ok := p.Args["sender"].(string); ok && sender != "" {
		opts.Sender = sender
	}

	// Get events in range
	events, _, err := s.store.ListEvents(p.Context, opts)
	if err != nil {
		s.logger.Error("Error listing events", "error", err)
		return nil, err
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("no events found in range")
	}

	// Replay each event
	replayedEvents := make([]*storage.Event, 0, len(events))
	for _, event := range events {
		replayEvent := &storage.Event{
			ID:           fmt.Sprintf("%s-replay-%s", event.ID, uuid.New().String()), // Format: original-id-replay-uuid
			Type:         event.Type,
			Payload:      event.Payload,
			Headers:      event.Headers,
			CreatedAt:    time.Now(),
			Status:       "replayed",
			Repository:   event.Repository,
			Sender:       event.Sender,
			ReplayedFrom: event.ID,
			OriginalTime: event.CreatedAt,
		}

		if err := s.store.StoreEvent(p.Context, replayEvent); err != nil {
			s.logger.Error("Error storing replayed event", "error", err)
			return nil, err
		}

		replayedEvents = append(replayedEvents, replayEvent)
	}

	return map[string]interface{}{
		"replayedCount": len(replayedEvents),
		"events":        replayedEvents,
	}, nil
}
