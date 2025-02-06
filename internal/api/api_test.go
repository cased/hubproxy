package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hubproxy/internal/api"
	"hubproxy/internal/storage"
	"hubproxy/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIHandler(t *testing.T) {
	// Initialize storage with clean test database
	store := testutil.SetupTestDB(t)
	defer store.Close()

	ctx := context.Background()

	now := time.Date(2025, 2, 6, 4, 20, 0, 0, time.UTC)

	// Create test events
	events := []*storage.Event{
		{
			ID:         "test-event-1",
			Type:       "push",
			Payload:    []byte(`{"ref": "refs/heads/main"}`),
			CreatedAt:  now.Add(-1 * time.Hour),
			Status:     "completed",
			Repository: "test/repo-1",
			Sender:     "user-1",
		},
		{
			ID:         "test-event-2",
			Type:       "pull_request",
			Payload:    []byte(`{"action": "opened"}`),
			CreatedAt:  now.Add(-2 * time.Hour),
			Status:     "pending",
			Repository: "test/repo-2",
			Sender:     "user-2",
		},
		{
			ID:         "test-event-3",
			Type:       "push",
			Payload:    []byte(`{"ref": "refs/heads/feature"}`),
			CreatedAt:  now,
			Status:     "completed",
			Repository: "test/repo-1",
			Sender:     "user-1",
		},
	}

	// Store test events
	for _, event := range events {
		err := store.StoreEvent(ctx, event)
		require.NoError(t, err)
	}

	// Create API handler
	logger := slog.New(slog.NewJSONHandler(nil, nil))
	handler := api.NewHandler(store, logger)

	t.Run("List Events", func(t *testing.T) {
		tests := []struct {
			name           string
			query          string
			expectedCount  int
			expectedStatus int
			validate       func(t *testing.T, events []*storage.Event)
		}{
			{
				name:           "List all events",
				query:          "",
				expectedCount:  3,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Filter by type",
				query:          "?type=push",
				expectedCount:  2,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Filter by repository",
				query:          "?repository=test/repo-1",
				expectedCount:  2,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Filter by sender",
				query:          "?sender=user-2",
				expectedCount:  1,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Filter by status",
				query:          "?status=pending",
				expectedCount:  1,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Filter by time range",
				query:          "?since=" + now.Add(-3*time.Hour).Format(time.RFC3339) + "&until=" + now.Format(time.RFC3339),
				expectedCount:  3,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Pagination - first page",
				query:          "?limit=2&offset=0",
				expectedCount:  2,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Pagination - second page",
				query:          "?limit=2&offset=2",
				expectedCount:  1,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Invalid time format",
				query:          "?since=invalid-time",
				expectedStatus: http.StatusBadRequest,
			},
			{
				name:           "Invalid limit",
				query:          "?limit=invalid",
				expectedStatus: http.StatusBadRequest,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(handler.ListEvents))
				defer server.Close()

				resp, err := http.Get(server.URL + tc.query)
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, tc.expectedStatus, resp.StatusCode)

				if tc.expectedStatus == http.StatusOK {
					var result struct {
						Events []*storage.Event `json:"events"`
						Total  int              `json:"total"`
					}
					err = json.NewDecoder(resp.Body).Decode(&result)
					require.NoError(t, err)

					assert.Equal(t, tc.expectedCount, len(result.Events))
					if tc.validate != nil {
						tc.validate(t, result.Events)
					}
				}
			})
		}
	})

	t.Run("Event Stats", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(handler.GetStats))
		defer server.Close()

		tests := []struct {
			name           string
			query          string
			expectedStats  map[string]int64
			expectedStatus int
		}{
			{
				name:  "All time stats",
				query: "",
				expectedStats: map[string]int64{
					"push":         2,
					"pull_request": 1,
				},
				expectedStatus: http.StatusOK,
			},
			{
				name:  "Stats with time range",
				query: "?since=" + now.Add(-3*time.Hour).Format(time.RFC3339),
				expectedStats: map[string]int64{
					"push":         2,
					"pull_request": 1,
				},
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Invalid time format",
				query:          "?since=invalid-time",
				expectedStatus: http.StatusBadRequest,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, err := http.Get(server.URL + tc.query)
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, tc.expectedStatus, resp.StatusCode)

				if tc.expectedStatus == http.StatusOK {
					var stats map[string]int64
					err = json.NewDecoder(resp.Body).Decode(&stats)
					require.NoError(t, err)
					assert.Equal(t, tc.expectedStats, stats)
				}
			})
		}
	})

	t.Run("Replay Events", func(t *testing.T) {
		tests := []struct {
			name           string
			handler        func(http.ResponseWriter, *http.Request)
			method         string
			path           string
			expectedStatus int
		}{
			{
				name:           "Replay single event",
				handler:        handler.ReplayEvent,
				method:         http.MethodPost,
				path:           "/api/events/test-event-1/replay",
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Replay non-existent event",
				handler:        handler.ReplayEvent,
				method:         http.MethodPost,
				path:           "/api/events/non-existent/replay",
				expectedStatus: http.StatusNotFound,
			},
			{
				name:           "Replay events by time range",
				handler:        handler.ReplayRange,
				method:         http.MethodPost,
				path:           "/api/replay?since=" + now.Add(-3*time.Hour).Format(time.RFC3339) + "&until=" + now.Format(time.RFC3339),
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Replay with invalid time range",
				handler:        handler.ReplayRange,
				method:         http.MethodPost,
				path:           "/api/replay?since=invalid",
				expectedStatus: http.StatusBadRequest,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(tc.handler))
				defer server.Close()

				req, err := http.NewRequest(tc.method, server.URL+tc.path, nil)
				require.NoError(t, err)

				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, tc.expectedStatus, resp.StatusCode)

				if tc.expectedStatus == http.StatusOK {
					var result struct {
						ReplayedCount int              `json:"replayed_count"`
						Events        []*storage.Event `json:"events,omitempty"`
					}
					err = json.NewDecoder(resp.Body).Decode(&result)
					require.NoError(t, err)

					if strings.Contains(tc.path, "/replay?") {
						assert.Greater(t, result.ReplayedCount, 0)
					} else {
						assert.Equal(t, 1, result.ReplayedCount)
					}
				}
			})
		}
	})
}
