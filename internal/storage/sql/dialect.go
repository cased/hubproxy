package sql

import "fmt"

// SQLDialect defines database-specific SQL syntax
type SQLDialect interface {
	// PlaceholderFormat returns the format for SQL placeholders ("?" or "$")
	PlaceholderFormat() string

	// JSONType returns the column type for storing JSON
	JSONType() string

	// TimeType returns the column type for storing timestamps
	TimeType() string

	// CreateTableSQL returns SQL for creating the events table
	CreateTableSQL(tableName string) string
}

// BaseDialect provides common implementations
type BaseDialect struct{}

// PlaceholderFormat returns "$" as the default placeholder format
func (d *BaseDialect) PlaceholderFormat() string {
	return "$"
}

// JSONType returns jsonb as the default JSON type
func (d *BaseDialect) JSONType() string {
	return "jsonb"
}

// TimeType returns timestamp as the default time type
func (d *BaseDialect) TimeType() string {
	return "timestamp"
}

// CreateTableSQL returns the default table creation SQL
func (d *BaseDialect) CreateTableSQL(tableName string) string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(36) PRIMARY KEY,
			type VARCHAR(50) NOT NULL,
			payload %s NOT NULL,
			headers %s,
			created_at %s NOT NULL,
			forwarded_at %s,
			error TEXT,
			repository VARCHAR(255),
			sender VARCHAR(255),
			replayed_from VARCHAR(255),
			original_time %s
		);
		CREATE INDEX IF NOT EXISTS idx_created_at ON %s (created_at);
		CREATE INDEX IF NOT EXISTS idx_forwarded_at ON %s (forwarded_at);
		CREATE INDEX IF NOT EXISTS idx_type ON %s (type);
		CREATE INDEX IF NOT EXISTS idx_repository ON %s (repository);
		CREATE INDEX IF NOT EXISTS idx_sender ON %s (sender);
		CREATE INDEX IF NOT EXISTS idx_replayed_from ON %s (replayed_from);
	`, tableName, d.JSONType(), d.JSONType(), d.TimeType(), d.TimeType(), d.TimeType(),
		tableName, tableName, tableName, tableName, tableName, tableName)
}
