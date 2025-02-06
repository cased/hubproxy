package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"log/slog"

	"hubproxy/internal/storage"
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
