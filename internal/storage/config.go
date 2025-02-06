package storage

// Config represents database configuration
type Config struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
}

func DefaultConfig() Config {
	return Config{
		Host:     "localhost",
		Port:     5433,
		Database: "lacrosse",
		Username: "lacrosse",
		Password: "lacrosse",
	}
}
