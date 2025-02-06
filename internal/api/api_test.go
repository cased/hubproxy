package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

	// Create test events
	events := []*storage.Event{
		{
			ID:         "test-event-1",
			Type:       "push",
			Payload:    []byte(`{"ref": "refs/heads/main"}`),
			CreatedAt:  time.Now().UTC(),
			Status:     "completed",
			Repository: "test/repo-1",
			Sender:     "user-1",
		},
		{
			ID:         "test-event-2",
			Type:       "pull_request",
			Payload:    []byte(`{"action": "opened"}`),
			CreatedAt:  time.Now().UTC(),
			Status:     "pending",
			Repository: "test/repo-2",
			Sender:     "user-2",
		},
	}

	for _, event := range events {
		err := store.StoreEvent(ctx, event)
		require.NoError(t, err)
	}

	// Create API handler
	logger := slog.New(slog.NewJSONHandler(nil, nil))
	handler := api.NewHandler(store, logger)

	tests := []struct {
		name           string
		query          string
		expectedCount  int
		expectedStatus int
	}{
		{
			name:           "List all events",
			query:          "",
			expectedCount:  2,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by type",
			query:          "?type=push",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by repository",
			query:          "?repository=test/repo-1",
			expectedCount:  1,
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create test server for each test
			server := httptest.NewServer(http.HandlerFunc(handler.ListEvents))
			defer server.Close()

			// Make request
			resp, err := http.Get(server.URL + tc.query)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Check status code
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectedStatus == http.StatusOK {
				var result struct {
					Events []*storage.Event `json:"events"`
					Total  int              `json:"total"`
				}
				err = json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)

				assert.Equal(t, tc.expectedCount, result.Total)
				assert.Len(t, result.Events, tc.expectedCount)
			}
		})
	}

	// Test stats endpoint
	t.Run("Event type stats", func(t *testing.T) {
		// Create test server for stats endpoint
		server := httptest.NewServer(http.HandlerFunc(handler.GetStats))
		defer server.Close()

		resp, err := http.Get(server.URL)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var stats map[string]int64
		err = json.NewDecoder(resp.Body).Decode(&stats)
		require.NoError(t, err)

		assert.Equal(t, int64(1), stats["push"])
		assert.Equal(t, int64(1), stats["pull_request"])
	})

	// Clean up test data
	_, total, err := store.ListEvents(ctx, storage.QueryOptions{
		Types: []string{"push", "pull_request"},
	})
	require.NoError(t, err)
	t.Logf("Cleaned up %d events", total)
}
