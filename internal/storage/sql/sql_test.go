package sql_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hubproxy/internal/storage"
	"hubproxy/internal/storage/sql"
)

func TestDuplicateEventHandling(t *testing.T) {
	ctx := context.Background()
	store, err := sql.New("sqlite:file:test.db?mode=memory&cache=shared")
	require.NoError(t, err)
	defer store.Close()

	// Create initial event
	event := &storage.Event{
		ID:         "test-event-1",
		Type:       "push",
		Payload:    []byte(`{"ref": "refs/heads/main"}`),
		Headers:    []byte(`{"X-GitHub-Event": ["push"], "X-GitHub-Delivery": ["test-event-1"]}`),
		CreatedAt:  time.Now().UTC(),
		Repository: "test/repo",
		Sender:     "test-user",
	}

	// Store the event first time
	err = store.StoreEvent(ctx, event)
	require.NoError(t, err)

	// Verify event was stored
	stored, err := store.GetEvent(ctx, event.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)

	// Try to store the same event with different status

	err = store.StoreEvent(ctx, event)
	require.NoError(t, err)

	// Verify the original event was not modified
	stored, err = store.GetEvent(ctx, event.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)

	// Count events to ensure no duplicates
	count, err := store.CountEvents(ctx, storage.QueryOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have exactly one event")
}

func TestConcurrentEventInsertion(t *testing.T) {
	ctx := context.Background()
	store, err := sql.New("sqlite:file:test_concurrent.db?mode=memory&cache=shared")
	require.NoError(t, err)
	defer store.Close()

	// Create base event
	event := &storage.Event{
		ID:         "concurrent-test-1",
		Type:       "push",
		Payload:    []byte(`{"ref": "refs/heads/main"}`),
		Headers:    []byte(`{"X-GitHub-Event": ["push"], "X-GitHub-Delivery": ["concurrent-test-1"]}`),
		CreatedAt:  time.Now().UTC(),
		Repository: "test/repo",
		Sender:     "test-user",
	}

	// Store the initial event
	err = store.StoreEvent(ctx, event)
	require.NoError(t, err)

	// Simulate concurrent insertions with the same ID
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			e := *event // Create a copy
			storeErr := store.StoreEvent(ctx, &e)
			require.NoError(t, storeErr)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify only one event exists
	count, err := store.CountEvents(ctx, storage.QueryOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have exactly one event despite concurrent insertions")

	// Verify the first event's status is unchanged (since duplicates are ignored)
	stored, err := store.GetEvent(ctx, event.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
}

func TestEventHeadersHandling(t *testing.T) {
	ctx := context.Background()
	store, err := sql.New("sqlite:file:test_headers.db?mode=memory&cache=shared")
	require.NoError(t, err)
	defer store.Close()

	// Create events with different headers
	events := []*storage.Event{
		{
			ID:         "test-headers-1",
			Type:       "push",
			Payload:    []byte(`{"ref": "refs/heads/main"}`),
			Headers:    []byte(`{"X-GitHub-Event": ["push"], "X-GitHub-Delivery": ["test-headers-1"]}`),
			CreatedAt:  time.Now().UTC(),
			Repository: "test/repo",
			Sender:     "test-user",
		},
		{
			ID:         "test-headers-2",
			Type:       "pull_request",
			Payload:    []byte(`{"action": "opened"}`),
			Headers:    []byte(`{"X-GitHub-Event": ["pull_request"], "X-GitHub-Delivery": ["test-headers-2"], "X-Custom-Header": ["test-value"]}`),
			CreatedAt:  time.Now().UTC(),
			Repository: "test/repo",
			Sender:     "test-user",
		},
	}

	// Store both events
	for _, event := range events {
		err = store.StoreEvent(ctx, event)
		require.NoError(t, err)
	}

	// Test GetEvent headers retrieval
	var stored *storage.Event
	for _, expected := range events {
		stored, err = store.GetEvent(ctx, expected.ID)
		require.NoError(t, err)
		require.NotNil(t, stored)
		assert.Equal(t, expected.Headers, stored.Headers, "Headers should match for event %s", expected.ID)
	}

	// Test ListEvents headers retrieval
	listed, total, err := store.ListEvents(ctx, storage.QueryOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, total, "Should have exactly two events")
	assert.Len(t, listed, 2, "Should list both events")

	// Create a map of expected events by ID for easier comparison
	expectedByID := make(map[string]*storage.Event)
	for _, e := range events {
		expectedByID[e.ID] = e
	}

	// Verify headers in listed events
	for _, actual := range listed {
		expected := expectedByID[actual.ID]
		require.NotNil(t, expected, "Should find matching expected event")
		assert.Equal(t, expected.Headers, actual.Headers, "Headers should match for event %s", actual.ID)
	}
}
