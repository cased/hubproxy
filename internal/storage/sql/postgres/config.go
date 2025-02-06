package postgres

// Config holds PostgreSQL connection configuration
type Config struct {
	dsn string // PostgreSQL DSN in format postgres://user:pass@host:port/dbname
}

// NewConfig creates a new PostgreSQL config
func NewConfig(dsn string) Config {
	return Config{dsn: dsn}
}

// DSN returns the data source name
func (c Config) DSN() string {
	return c.dsn
}
