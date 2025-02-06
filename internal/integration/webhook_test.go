package integration

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hubproxy/internal/webhook"
)

func TestWebhookIntegration(t *testing.T) {
	// Test configuration
	secret := "test-secret"
	payload := []byte(`{"action": "test", "repository": {"full_name": "test/repo"}}`)

	// Initialize test database
	store := SetupTestDB(t)
	defer store.Close()

	// Create a mock target server to receive forwarded webhooks
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the forwarded request
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, payload, body)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "push", r.Header.Get("X-GitHub-Event"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Create webhook handler
	handler := webhook.NewHandler(webhook.Options{
		Secret:    secret,
		TargetURL: ts.URL,
		Logger:    slog.New(slog.NewJSONHandler(os.Stdout, nil)),
		Store:     store,
	})

	// Create test server with webhook handler
	server := httptest.NewServer(handler)
	defer server.Close()

	t.Run("Valid webhook request", func(t *testing.T) {
		// Calculate signature
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		// Create request
		req, err := http.NewRequest("POST", server.URL, bytes.NewReader(payload))
		require.NoError(t, err)

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-GitHub-Delivery", "test-delivery-id")
		req.Header.Set("X-Hub-Signature-256", signature)

		// Send request
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Invalid signature", func(t *testing.T) {
		// Create request with invalid signature
		req, err := http.NewRequest("POST", server.URL, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-GitHub-Delivery", "test-delivery-id")
		req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Missing event type", func(t *testing.T) {
		// Calculate signature
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		// Create request without event type header
		req, err := http.NewRequest("POST", server.URL, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Hub-Signature-256", signature)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Invalid method", func(t *testing.T) {
		// Try GET request
		req, err := http.NewRequest("GET", server.URL, nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})
}
