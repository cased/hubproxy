package sql

// SQLiteDialect implements SQLDialect for SQLite
type SQLiteDialect struct {
	BaseDialect
}

func (d *SQLiteDialect) PlaceholderFormat() string {
	return "?"
}

func (d *SQLiteDialect) JSONType() string {
	return "TEXT"
}

func (d *SQLiteDialect) TimeType() string {
	return "DATETIME"
}

func (d *SQLiteDialect) InsertIgnoreSQL() string {
	return "INSERT OR IGNORE"
}

// PostgresDialect implements SQLDialect for PostgreSQL
type PostgresDialect struct {
	BaseDialect
}

func (d *PostgresDialect) PlaceholderFormat() string {
	return "$"
}

func (d *PostgresDialect) JSONType() string {
	return "JSONB"
}

func (d *PostgresDialect) TimeType() string {
	return "TIMESTAMP WITH TIME ZONE"
}

func (d *PostgresDialect) InsertIgnoreSQL() string {
	return "INSERT" //Will be used with ON CONFLICT DO NOTHING
}

// MySQLDialect implements SQLDialect for MySQL
type MySQLDialect struct {
	BaseDialect
}

func (d *MySQLDialect) PlaceholderFormat() string {
	return "?"
}

func (d *MySQLDialect) JSONType() string {
	return "JSON"
}

func (d *MySQLDialect) TimeType() string {
	return "DATETIME"
}

func (d *MySQLDialect) InsertIgnoreSQL() string {
	return "INSERT IGNORE"
}
