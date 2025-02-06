package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"hubproxy/internal/storage"
	"hubproxy/internal/storage/sql/sqlite"
)

// SetupTestDB creates a temporary SQLite database for testing
func SetupTestDB(t testing.TB) storage.Storage {
	// Create temp dir for test database
	tmpDir, err := os.MkdirTemp("", "hubproxy-test-*")
	require.NoError(t, err)

	// Register cleanup
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	// Create SQLite database
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := sqlite.NewStorage(dbPath)
	require.NoError(t, err)

	// Register store cleanup
	t.Cleanup(func() {
		store.Close()
	})

	// Create schema
	ctx := context.Background()
	err = store.CreateSchema(ctx)
	require.NoError(t, err)

	return store
}
