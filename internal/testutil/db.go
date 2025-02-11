package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"hubproxy/internal/storage"
	"hubproxy/internal/storage/factory"
)

func NewTestDB(t *testing.T) storage.Storage {
	t.Helper()

	// Create a temporary directory for the SQLite database
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create a new SQLite database
	store, err := factory.NewStorageFromURI("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Register cleanup function
	t.Cleanup(func() {
		store.Close()
		os.Remove(dbPath)
	})

	return store
}
