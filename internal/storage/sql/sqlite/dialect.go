package sqlite

import (
	"fmt"
	"hubproxy/internal/storage/sql"
)

// Dialect implements SQLite-specific SQL dialect
type Dialect struct {
	*sql.BaseDialect
}

// NewDialect creates a new SQLite dialect
func NewDialect() *Dialect {
	return &Dialect{
		BaseDialect: &sql.BaseDialect{},
	}
}

// PlaceholderFormat returns "?" as SQLite uses ? for placeholders
func (d *Dialect) PlaceholderFormat() string {
	return "?"
}

// JSONType returns TEXT as SQLite's JSON type (stored as text)
func (d *Dialect) JSONType() string {
	return "TEXT"
}

// TimeType returns DATETIME as SQLite's timestamp type
func (d *Dialect) TimeType() string {
	return "DATETIME"
}

// CreateTableSQL returns SQLite-specific table creation SQL
func (d *Dialect) CreateTableSQL(tableName string) string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			payload %s NOT NULL,
			created_at %s NOT NULL,
			status TEXT NOT NULL,
			error TEXT,
			repository TEXT,
			sender TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_created_at ON %s (created_at);
		CREATE INDEX IF NOT EXISTS idx_type ON %s (type);
		CREATE INDEX IF NOT EXISTS idx_status ON %s (status);
		CREATE INDEX IF NOT EXISTS idx_repository ON %s (repository);
		CREATE INDEX IF NOT EXISTS idx_sender ON %s (sender);
	`, tableName, d.JSONType(), d.TimeType(),
		tableName, tableName, tableName, tableName, tableName)
}
