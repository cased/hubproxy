package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/tsnet"

	"hubproxy/internal/storage"
	"hubproxy/internal/webhook"
)

// calculateSignature calculates the GitHub webhook signature
func calculateSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))
	return "sha256=" + signature
}

func TestTailscaleIntegration(t *testing.T) {
	// Initialize test database
	store := SetupTestDB(t)
	defer store.Close()

	// Create a test target server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		defer r.Body.Close()

		// Echo back the request details
		fmt.Fprintf(w, "Received %s request with body: %s", r.Method, string(body))
	}))
	defer targetServer.Close()

	// Create a logger that discards output during tests
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	const secret = "test-secret"

	// Create webhook handler
	handler := webhook.NewHandler(webhook.Options{
		Secret:           secret,
		TargetURL:        targetServer.URL,
		Store:            store,
		Logger:           logger,
		ValidateIP:       false,
		MetricsCollector: storage.NewDBMetricsCollector(store, logger),
	})

	// Create HTTP mux
	mux := http.NewServeMux()
	mux.Handle("/", handler)

	t.Run("Regular server", func(t *testing.T) {
		// Test regular HTTP server
		server := httptest.NewServer(mux)
		defer server.Close()

		// Send a test webhook
		payload := `{"event": "test"}`
		req, err := http.NewRequest("POST", server.URL, strings.NewReader(payload))
		require.NoError(t, err)

		// Calculate correct signature
		signature := calculateSignature(secret, []byte(payload))
		req.Header.Set("X-Hub-Signature-256", signature)
		req.Header.Set("X-GitHub-Event", "test")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Tailscale server", func(t *testing.T) {
		// Skip if no auth key provided
		authKey := os.Getenv("TAILSCALE_TEST_AUTHKEY")
		if authKey == "" {
			t.Skip("Skipping Tailscale test - no auth key provided")
		}

		// Create Tailscale server
		ts := &tsnet.Server{
			Hostname: "hubproxy-test",
			AuthKey:  authKey,
		}
		defer ts.Close()

		// Create listener
		ln, err := ts.Listen("tcp", ":0")
		require.NoError(t, err)

		// Start server
		go http.Serve(ln, mux)

		// Wait for server to be ready
		time.Sleep(2 * time.Second)

		// Get server address
		addr := ln.Addr().String()

		// Send a test webhook
		payload := `{"event": "test"}`
		url := fmt.Sprintf("http://%s", addr)
		req, err := http.NewRequest("POST", url, strings.NewReader(payload))
		require.NoError(t, err)

		// Calculate correct signature
		signature := calculateSignature(secret, []byte(payload))
		req.Header.Set("X-Hub-Signature-256", signature)
		req.Header.Set("X-GitHub-Event", "test")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
