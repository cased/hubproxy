package main

import (
	"os"
	"testing"

	"hubproxy/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigPrecedence(t *testing.T) {
	// Set test webhook secret
	oldSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	os.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret")
	defer func() {
		if oldSecret != "" {
			os.Setenv("GITHUB_WEBHOOK_SECRET", oldSecret)
		} else {
			os.Unsetenv("GITHUB_WEBHOOK_SECRET")
		}
	}()

	// Create a temporary config file
	content := []byte(`
target_url: "http://config-file:8080"
log_level: "debug"
validate_ip: true
ts_hostname: "config-host"
db_type: "sqlite"
db_dsn: ":memory:"
`)

	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write(content)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	// Test cases to verify precedence
	tests := []struct {
		name    string
		args    []string
		wantCfg config.Config
		wantErr bool
	}{
		{
			name: "config file only",
			args: []string{"--config", tmpfile.Name(), "--test-mode"},
			wantCfg: config.Config{
				TargetURL:  "http://config-file:8080",
				LogLevel:   "debug",
				ValidateIP: true,
				TSHostname: "config-host",
				DBType:     "sqlite",
				DBDSN:      ":memory:",
			},
		},
		{
			name: "flags override config",
			args: []string{
				"--config", tmpfile.Name(),
				"--target", "http://flag-override:9090",
				"--log-level", "info",
				"--validate-ip=false",
				"--ts-hostname", "flag-host",
				"--db", "postgres",
				"--db-dsn", "postgres://localhost:5432/test",
				"--test-mode",
			},
			wantCfg: config.Config{
				TargetURL:  "http://flag-override:9090",
				LogLevel:   "info",
				ValidateIP: false,
				TSHostname: "flag-host",
				DBType:     "postgres",
				DBDSN:      "postgres://localhost:5432/test",
			},
		},
		{
			name: "partial flag override",
			args: []string{
				"--config", tmpfile.Name(),
				"--target", "http://partial-override:9090",
				"--log-level", "warn",
				"--test-mode",
			},
			wantCfg: config.Config{
				TargetURL:  "http://partial-override:9090",
				LogLevel:   "warn",
				ValidateIP: true,
				TSHostname: "config-host",
				DBType:     "sqlite",
				DBDSN:      ":memory:",
			},
		},
		{
			name: "flags only",
			args: []string{
				"--target", "http://flags-only:9090",
				"--log-level", "error",
				"--validate-ip=false",
				"--ts-hostname", "flags-host",
				"--db", "mysql",
				"--db-dsn", "user:pass@tcp(localhost:3306)/test",
				"--test-mode",
			},
			wantCfg: config.Config{
				TargetURL:  "http://flags-only:9090",
				LogLevel:   "error",
				ValidateIP: false,
				TSHostname: "flags-host",
				DBType:     "mysql",
				DBDSN:      "user:pass@tcp(localhost:3306)/test",
			},
		},
		{
			name:    "invalid config file",
			args:    []string{"--config", "nonexistent.yaml", "--test-mode"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global config
			cfg = config.Config{}
			configFile = ""

			// Create command
			cmd := newRootCmd()
			cmd.SetArgs(tt.args)

			// Execute command
			err := cmd.Execute()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCfg.TargetURL, cfg.TargetURL)
			assert.Equal(t, tt.wantCfg.LogLevel, cfg.LogLevel)
			assert.Equal(t, tt.wantCfg.ValidateIP, cfg.ValidateIP)
			assert.Equal(t, tt.wantCfg.TSHostname, cfg.TSHostname)
			assert.Equal(t, tt.wantCfg.DBType, cfg.DBType)
			assert.Equal(t, tt.wantCfg.DBDSN, cfg.DBDSN)
		})
	}
}
