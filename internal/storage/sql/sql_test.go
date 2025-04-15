package sql

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hubproxy/internal/storage"
)

func TestDuplicateEventHandling(t *testing.T) {
	ctx := context.Background()
	store, err := New("sqlite::memory:")
	require.NoError(t, err)
	defer store.Close()

	// Create initial event
	event := &storage.Event{
		ID:         "test-event-1",
		Type:       "push",
		Payload:    []byte(`{"ref": "refs/heads/main"}`),
		CreatedAt:  time.Now().UTC(),
		Status:     "pending",
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
	assert.Equal(t, "pending", stored.Status)

	// Try to store the same event with different status
	event.Status = "completed"
	err = store.StoreEvent(ctx, event)
	require.NoError(t, err)

	// Verify the original event was not modified
	stored, err = store.GetEvent(ctx, event.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "pending", stored.Status, "Original event should not be modified")

	// Count events to ensure no duplicates
	count, err := store.CountEvents(ctx, storage.QueryOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have exactly one event")
}

func TestConcurrentEventInsertion(t *testing.T) {
	ctx := context.Background()
	store, err := New("sqlite::memory:")
	require.NoError(t, err)
	defer store.Close()

	// Create base event
	event := &storage.Event{
		ID:         "concurrent-test-1",
		Type:       "push",
		Payload:    []byte(`{"ref": "refs/heads/main"}`),
		CreatedAt:  time.Now().UTC(),
		Status:     "pending",
		Repository: "test/repo",
		Sender:     "test-user",
	}

	// Simulate concurrent insertions
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(status string) {
			e := *event // Create a copy
			e.Status = status
			storeErr := store.StoreEvent(ctx, &e)
			require.NoError(t, storeErr)
			done <- true
		}(fmt.Sprintf("status-%d", i))
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify only one event exists
	count, err := store.CountEvents(ctx, storage.QueryOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have exactly one event despite concurrent insertions")

	// Verify the first event's status remained unchanged
	stored, err := store.GetEvent(ctx, event.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "pending", stored.Status, "Original event status should be unchanged")
}
