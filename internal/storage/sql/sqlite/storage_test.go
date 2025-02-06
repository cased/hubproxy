package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hubproxy/internal/storage"
)

func TestSQLiteStorage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hubproxy-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStorage(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create schema
	err = store.CreateSchema(ctx)
	require.NoError(t, err)

	// Test event creation
	event := &storage.Event{
		ID:         uuid.New().String(),
		Type:       "push",
		Payload:    []byte(`{"ref": "refs/heads/main"}`),
		CreatedAt:  time.Now().UTC(),
		Status:     "pending",
		Error:      "", // Empty string for no error
		Repository: "test/repo",
		Sender:     "test-user",
	}

	err = store.StoreEvent(ctx, event)
	require.NoError(t, err)

	// Test event update
	event.Status = "completed"
	event.Error = "no error"
	err = store.StoreEvent(ctx, event)
	require.NoError(t, err)

	// Verify the update worked
	events, total, err := store.ListEvents(ctx, storage.QueryOptions{
		Types:      []string{"push"},
		Repository: "test/repo",
		Sender:     "test-user",
		Status:     "completed",
		Limit:      10,
		Offset:     0,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, events, 1)
	assert.Equal(t, event.ID, events[0].ID)
	assert.Equal(t, "completed", events[0].Status)
	assert.Equal(t, "no error", events[0].Error)
}
