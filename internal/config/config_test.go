package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromFile(t *testing.T) {
	// Create a temporary config file
	content := []byte(`
target_url: "http://localhost:8080"
log_level: "debug"
validate_ip: true
ts_hostname: "test-host"
db_type: "sqlite"
db_dsn: "test.db"
`)

	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write(content)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	// Test loading the config
	cfg, err := LoadFromFile(tmpfile.Name())
	require.NoError(t, err)

	// Verify loaded values
	assert.Equal(t, "http://localhost:8080", cfg.TargetURL)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.True(t, cfg.ValidateIP)
	assert.Equal(t, "test-host", cfg.TSHostname)
	assert.Equal(t, "sqlite", cfg.DBType)
	assert.Equal(t, "test.db", cfg.DBDSN)
}

func TestLoadFromFile_Defaults(t *testing.T) {
	// Create a minimal config file
	content := []byte(`
target_url: "http://localhost:8080"
`)

	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write(content)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	// Test loading the config
	cfg, err := LoadFromFile(tmpfile.Name())
	require.NoError(t, err)

	// Verify default values
	assert.Equal(t, "info", cfg.LogLevel)
	assert.True(t, cfg.ValidateIP)
	assert.Equal(t, "hubproxy", cfg.TSHostname)
	assert.Equal(t, "sqlite", cfg.DBType)
	assert.Equal(t, "hubproxy.db", cfg.DBDSN)
}

func TestLoadFromFile_FileNotFound(t *testing.T) {
	_, err := LoadFromFile(filepath.Join(os.TempDir(), "nonexistent.yaml"))
	assert.Error(t, err)
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	// Create an invalid YAML file
	content := []byte(`
target_url: http://localhost:8080
invalid yaml content
`)

	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write(content)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	_, err = LoadFromFile(tmpfile.Name())
	assert.Error(t, err)
}

func TestGetSecret(t *testing.T) {
	// Test with environment variable set
	const testSecret = "test-webhook-secret"
	t.Setenv("GITHUB_WEBHOOK_SECRET", testSecret)
	assert.Equal(t, testSecret, GetSecret())

	// Test with environment variable unset
	t.Setenv("GITHUB_WEBHOOK_SECRET", "")
	assert.Empty(t, GetSecret())
}
