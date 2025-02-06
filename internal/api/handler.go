package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"hubproxy/internal/storage"
	"github.com/google/uuid"
)

// Handler handles API requests
type Handler struct {
	store  storage.Storage
	logger *slog.Logger
}

// NewHandler creates a new API handler
func NewHandler(store storage.Storage, logger *slog.Logger) *Handler {
	return &Handler{
		store:  store,
		logger: logger,
	}
}

// ListEvents handles GET /api/events
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	opts := storage.QueryOptions{
		Limit:  50, // Default limit
		Offset: 0,  // Default offset
	}

	// Parse type filter
	if t := query.Get("type"); t != "" {
		opts.Types = []string{t} // Single type for now
	}

	// Parse other filters
	opts.Repository = query.Get("repository")
	opts.Sender = query.Get("sender")
	opts.Status = query.Get("status")

	// Parse since/until
	if since := query.Get("since"); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			http.Error(w, "Invalid since parameter", http.StatusBadRequest)
			return
		}
		opts.Since = t
	}

	if until := query.Get("until"); until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err != nil {
			http.Error(w, "Invalid until parameter", http.StatusBadRequest)
			return
		}
		opts.Until = t
	}

	// Parse limit/offset
	if limit := query.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil {
			http.Error(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
		opts.Limit = n
	}

	if offset := query.Get("offset"); offset != "" {
		n, err := strconv.Atoi(offset)
		if err != nil {
			http.Error(w, "Invalid offset parameter", http.StatusBadRequest)
			return
		}
		opts.Offset = n
	}

	// Get events
	events, total, err := h.store.ListEvents(r.Context(), opts)
	if err != nil {
		h.logger.Error("Error listing events", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
		"total":  total,
	}); err != nil {
		h.logger.Error("Error encoding response", "error", err)
	}
}

// GetStats handles GET /api/stats
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get stats for last 24 hours by default
	since := time.Now().Add(-24 * time.Hour)
	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			http.Error(w, "Invalid since parameter", http.StatusBadRequest)
			return
		}
		since = t
	}

	stats, err := h.store.GetEventTypeStats(r.Context(), since)
	if err != nil {
		h.logger.Error("Error getting stats", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		h.logger.Error("Error encoding response", "error", err)
	}
}

// ReplayEvent handles POST /api/events/:id/replay
func (h *Handler) ReplayEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract event ID from path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 4 || parts[3] != "replay" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Get event from storage
	opts := storage.QueryOptions{
		Types:  []string{},
		Limit:  1,
		Offset: 0,
	}
	events, _, err := h.store.ListEvents(r.Context(), opts)
	if err != nil {
		h.logger.Error("Error getting event", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if len(events) == 0 {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}
	event := events[0]

	// Create new event with same payload but new ID and timestamp
	replayEvent := &storage.Event{
		ID:           fmt.Sprintf("%s-replay-%s", event.ID, uuid.New().String()),  // Format: original-id-replay-uuid
		Type:         event.Type,
		Payload:      event.Payload,
		CreatedAt:    time.Now(),
		Status:       "replayed",
		Repository:   event.Repository,
		Sender:       event.Sender,
		ReplayedFrom: event.ID,
		OriginalTime: event.CreatedAt,
	}

	// Store the replayed event
	if err := h.store.StoreEvent(r.Context(), replayEvent); err != nil {
		h.logger.Error("Error storing replayed event", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(replayEvent); err != nil {
		h.logger.Error("Error encoding response", "error", err)
	}
}

// ReplayRange handles POST /api/replay with time range parameters
func (h *Handler) ReplayRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters for time range
	query := r.URL.Query()
	opts := storage.QueryOptions{
		Limit:  100, // Default limit for replay
		Offset: 0,
	}

	// Parse since/until (both required for range replay)
	since := query.Get("since")
	if since == "" {
		http.Error(w, "Missing since parameter", http.StatusBadRequest)
		return
	}
	sinceTime, err := time.Parse(time.RFC3339, since)
	if err != nil {
		http.Error(w, "Invalid since parameter", http.StatusBadRequest)
		return
	}
	opts.Since = sinceTime

	until := query.Get("until")
	if until == "" {
		http.Error(w, "Missing until parameter", http.StatusBadRequest)
		return
	}
	untilTime, err := time.Parse(time.RFC3339, until)
	if err != nil {
		http.Error(w, "Invalid until parameter", http.StatusBadRequest)
		return
	}
	opts.Until = untilTime

	// Optional filters
	if t := query.Get("type"); t != "" {
		opts.Types = []string{t}
	}
	if repo := query.Get("repository"); repo != "" {
		opts.Repository = repo
	}
	if sender := query.Get("sender"); sender != "" {
		opts.Sender = sender
	}

	// Get events in range
	events, _, err := h.store.ListEvents(r.Context(), opts)
	if err != nil {
		h.logger.Error("Error listing events", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if len(events) == 0 {
		http.Error(w, "No events found in range", http.StatusNotFound)
		return
	}

	// Replay each event
	replayedEvents := make([]*storage.Event, 0, len(events))
	for _, event := range events {
		replayEvent := &storage.Event{
			ID:           fmt.Sprintf("%s-replay-%s", event.ID, uuid.New().String()),  // Format: original-id-replay-uuid
			Type:         event.Type,
			Payload:      event.Payload,
			CreatedAt:    time.Now(),
			Status:       "replayed",
			Repository:   event.Repository,
			Sender:       event.Sender,
			ReplayedFrom: event.ID,
			OriginalTime: event.CreatedAt,
		}

		if err := h.store.StoreEvent(r.Context(), replayEvent); err != nil {
			h.logger.Error("Error storing replayed event", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		replayedEvents = append(replayedEvents, replayEvent)
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"replayed_count": len(replayedEvents),
		"events":         replayedEvents,
	}); err != nil {
		h.logger.Error("Error encoding response", "error", err)
	}
}
