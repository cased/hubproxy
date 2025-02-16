package integration

import (
	"testing"

	"github.com/stretchr/testify/require"

	"hubproxy/internal/storage"
	"hubproxy/internal/storage/sql"
)

// SetupTestDB creates a new SQLite in-memory database for testing
func SetupTestDB(t testing.TB) storage.Storage {
	t.Helper()

	store, err := sql.New("sqlite://:memory:")
	require.NoError(t, err)

	t.Cleanup(func() {
		store.Close()
	})

	return store
}
