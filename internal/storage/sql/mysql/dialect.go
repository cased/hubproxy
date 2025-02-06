package mysql

import (
	"fmt"
	"hubproxy/internal/storage/sql"
)

// Dialect implements MySQL-specific SQL dialect
type Dialect struct {
	*sql.BaseDialect
}

// NewDialect creates a new MySQL dialect
func NewDialect() *Dialect {
	return &Dialect{
		BaseDialect: &sql.BaseDialect{},
	}
}

// PlaceholderFormat returns "?" as MySQL uses ? for placeholders
func (d *Dialect) PlaceholderFormat() string {
	return "?"
}

// JSONType returns JSON as MySQL's JSON type
func (d *Dialect) JSONType() string {
	return "JSON"
}

// TimeType returns DATETIME as MySQL's timestamp type
func (d *Dialect) TimeType() string {
	return "DATETIME"
}

// CreateTableSQL returns MySQL-specific table creation SQL
func (d *Dialect) CreateTableSQL(tableName string) string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(36) PRIMARY KEY,
			type VARCHAR(50) NOT NULL,
			payload %s NOT NULL,
			created_at %s NOT NULL,
			status VARCHAR(20) NOT NULL,
			error TEXT,
			repository VARCHAR(255),
			sender VARCHAR(255),
			INDEX idx_created_at (created_at),
			INDEX idx_type (type),
			INDEX idx_status (status),
			INDEX idx_repository (repository),
			INDEX idx_sender (sender)
		)
	`, tableName, d.JSONType(), d.TimeType())
}
