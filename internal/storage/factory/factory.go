package factory

import (
	"fmt"
	"strings"

	"github.com/xo/dburl"

	"hubproxy/internal/storage"
	"hubproxy/internal/storage/sql/mysql"
	"hubproxy/internal/storage/sql/postgres"
	"hubproxy/internal/storage/sql/sqlite"
)

// NewStorageFromURI creates a new storage instance from a database URI.
// The URI format follows the dburl package conventions:
//   - SQLite: sqlite:/path/to/file.db or sqlite:file.db
//   - MySQL: mysql://user:pass@host/dbname
//   - PostgreSQL: postgres://user:pass@host/dbname
func NewStorageFromURI(uri string) (storage.Storage, error) {
	u, err := dburl.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}

	cfg := storage.Config{
		Host:     u.Hostname(),
		Database: strings.TrimPrefix(u.Path, "/"),
		Username: u.User.Username(),
	}
	if password, ok := u.User.Password(); ok {
		cfg.Password = password
	}
	if u.Port() != "" {
		if _, err := fmt.Sscanf(u.Port(), "%d", &cfg.Port); err != nil {
			return nil, fmt.Errorf("parsing port: %w", err)
		}
	}

	switch u.Driver {
	case "sqlite3", "sqlite":
		return sqlite.NewStorage(strings.TrimPrefix(u.DSN, "file:"))
	case "mysql":
		if cfg.Port == 0 {
			cfg.Port = 3306 // default MySQL port
		}
		return mysql.NewStorage(cfg)
	case "postgres", "postgresql":
		if cfg.Port == 0 {
			cfg.Port = 5432 // default PostgreSQL port
		}
		return postgres.NewStorage(cfg)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", u.Driver)
	}
}
