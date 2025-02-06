package mysql

// Config holds MySQL connection configuration
type Config struct {
	dsn string // MySQL DSN in format user:pass@tcp(host:port)/dbname
}

// NewConfig creates a new MySQL config
func NewConfig(dsn string) Config {
	return Config{dsn: dsn}
}

// DSN returns the data source name
func (c Config) DSN() string {
	return c.dsn
}
