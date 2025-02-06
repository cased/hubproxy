package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration
type Config struct {
	TargetURL  string `yaml:"target_url"`
	LogLevel   string `yaml:"log_level"`
	ValidateIP bool   `yaml:"validate_ip"`
	TSAuthKey  string `yaml:"ts_authkey"`
	TSHostname string `yaml:"ts_hostname"`
	DBType     string `yaml:"db_type"`
	DBDSN      string `yaml:"db_dsn"`
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	// ValidateIP defaults to true if not specified
	if !cfg.ValidateIP {
		cfg.ValidateIP = true
	}
	if cfg.TSHostname == "" {
		cfg.TSHostname = "hubproxy"
	}
	if cfg.DBType == "" {
		cfg.DBType = "sqlite"
	}
	if cfg.DBDSN == "" {
		cfg.DBDSN = "hubproxy.db"
	}

	return &cfg, nil
}

// GetSecret returns the webhook secret from environment variable
func GetSecret() string {
	return os.Getenv("GITHUB_WEBHOOK_SECRET")
}
