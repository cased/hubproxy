package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hubproxy/internal/storage"
	"hubproxy/internal/testutil"
)

func TestStorageImplementations(t *testing.T) {
	store := testutil.SetupTestDB(t)
	ctx := context.Background()

	// Test event creation
	event := &storage.Event{
		ID:         "test-event-1",
		Type:       "push",
		Payload:    []byte(`{"ref": "refs/heads/main"}`),
		CreatedAt:  time.Now().UTC(),
		Status:     "pending",
		Error:      "", // Empty string for no error
		Repository: "test/repo",
		Sender:     "test-user",
	}

	err := store.StoreEvent(ctx, event)
	require.NoError(t, err)

	// Create a pull_request event
	pullRequestEvent := &storage.Event{
		ID:         "test-event-2",
		Type:       "pull_request",
		Payload:    []byte(`{"action": "opened"}`),
		CreatedAt:  time.Now().UTC(),
		Status:     "completed",
		Error:      "",
		Repository: "test/repo",
		Sender:     "test-user",
	}

	err = store.StoreEvent(ctx, pullRequestEvent)
	require.NoError(t, err)

	// Test event update by storing with same ID
	event.Status = "completed"
	event.Error = "no error"
	err = store.StoreEvent(ctx, event)
	require.NoError(t, err)

	// Test event listing with single event
	events, total, err := store.ListEvents(ctx, storage.QueryOptions{
		Types:      []string{"push"},
		Repository: "test/repo",
		Sender:     "test-user",
		Status:     "completed", // Should match updated status
		Limit:      10,
		Offset:     0,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, events, 1)
	assert.Equal(t, event.ID, events[0].ID)
	assert.Equal(t, "completed", events[0].Status)
	assert.Equal(t, "no error", events[0].Error)

	// Test getting event type stats
	t.Run("GetEventTypeStats", func(t *testing.T) {
		stats, statsErr := store.GetStats(ctx, time.Time{})
		require.NoError(t, statsErr)
		require.NotNil(t, stats)

		// Verify stats
		require.Equal(t, int64(1), stats["push"])
		require.Equal(t, int64(1), stats["pull_request"])
	})

	// Test count events
	count, err := store.CountEvents(ctx, storage.QueryOptions{
		Types: []string{"push"},
		Since: time.Now().Add(-24 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
