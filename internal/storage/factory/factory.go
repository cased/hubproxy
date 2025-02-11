package factory

import (
	"context"
	"fmt"

	"github.com/xo/dburl"

	"hubproxy/internal/storage"
	"hubproxy/internal/storage/sql"
)

// NewStorageFromURI creates a new storage instance from a database URI.
// The URI format follows the dburl package conventions:
//   - SQLite: sqlite:/path/to/file.db or sqlite:file.db
//   - MySQL: mysql://user:pass@host/dbname
//   - PostgreSQL: postgres://user:pass@host/dbname
func NewStorageFromURI(uri string) (storage.Storage, error) {
	// Parse the URL to validate it
	_, err := dburl.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}

	// Create storage using the unified SQL implementation
	store, err := sql.NewStorage(context.Background(), uri)
	if err != nil {
		return nil, fmt.Errorf("creating storage: %w", err)
	}

	// Create schema if needed
	if err := store.CreateSchema(context.Background()); err != nil {
		store.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return store, nil
}
