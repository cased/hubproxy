package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"hubproxy/internal/storage"
	"hubproxy/internal/storage/sql/sqlite"
)

// SetupTestDB creates a new SQLite in-memory database for testing
func SetupTestDB(t testing.TB) storage.Storage {
	t.Helper()

	store, err := sqlite.NewStorage(":memory:")
	require.NoError(t, err)

	err = store.CreateSchema(context.Background())
	require.NoError(t, err)

	t.Cleanup(func() {
		store.Close()
	})

	return store
}
