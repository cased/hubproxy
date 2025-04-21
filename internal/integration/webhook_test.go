package integration

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hubproxy/internal/storage"
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

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Create and start metrics collector
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	metricsCollector := storage.NewDBMetricsCollector(store, logger)
	metricsCollector.StartMetricsCollection(ctx, time.Second)

	// Create webhook handler and forwarder
	handler := webhook.NewHandler(webhook.Options{
		Secret:           secret,
		Logger:           logger,
		Store:            store,
		MetricsCollector: metricsCollector,
	})

	forwarder := webhook.NewWebhookForwarder(webhook.WebhookForwarderOptions{
		TargetURL:        ts.URL,
		Storage:          store,
		MetricsCollector: metricsCollector,
		Logger:           logger,
	})

	// Start the forwarder
	go forwarder.StartForwarder(ctx)

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

		// Wait a bit for the event to be stored
		time.Sleep(100 * time.Millisecond)

		// Get the event from storage to verify it was stored
		events, _, err := store.ListEvents(ctx, storage.QueryOptions{})
		require.NoError(t, err)
		require.Len(t, events, 1)

		// Verify forwarded_at is null before processing
		event, err := store.GetEvent(ctx, events[0].ID)
		require.NoError(t, err)
		require.NotNil(t, event)
		assert.Nil(t, event.ForwardedAt, "forwarded_at should be null before processing")

		// Trigger event processing
		err = forwarder.ProcessEvents(ctx)
		require.NoError(t, err)

		// Wait a bit for processing to complete
		time.Sleep(100 * time.Millisecond)

		// Verify forwarded_at is set after processing
		event, err = store.GetEvent(ctx, events[0].ID)
		require.NoError(t, err)
		require.NotNil(t, event)
		assert.NotNil(t, event.ForwardedAt, "forwarded_at should be set after processing")
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

func TestWebhookUnixSocket(t *testing.T) {
	// Test configuration
	secret := "test-secret"
	payload := []byte(`{"action": "test", "repository": {"full_name": "test/repo"}}`)
	socketPath := "/tmp/hubproxy-test.sock"

	// Initialize test database
	store := SetupTestDB(t)
	defer store.Close()

	// Clean up socket file if it exists
	os.Remove(socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()
	defer os.Remove(socketPath)

	// Channel to receive forwarded request data
	receivedCh := make(chan []byte, 1)

	// Start Unix socket server
	go func() {
		conn, errAccept := listener.Accept()
		if errAccept != nil {
			t.Logf("Error accepting connection: %v", errAccept)
			return
		}
		defer conn.Close()

		// Read HTTP request
		bufReader := bufio.NewReader(conn)
		req, errRead := http.ReadRequest(bufReader)
		if errRead != nil {
			t.Logf("Error reading request: %v", errRead)
			return
		}

		// Verify headers
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		assert.Equal(t, "push", req.Header.Get("X-GitHub-Event"))

		// Read body
		body, errBody := io.ReadAll(req.Body)
		if errBody != nil {
			t.Logf("Error reading body: %v", errBody)
			return
		}

		// Send response
		resp := http.Response{
			StatusCode: http.StatusOK,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
		}
		resp.Write(conn)

		// Send received data to channel
		receivedCh <- body
	}()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Create and start metrics collector
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	metricsCollector := storage.NewDBMetricsCollector(store, logger)
	metricsCollector.StartMetricsCollection(ctx, time.Second)

	// Create webhook handler and forwarder
	handler := webhook.NewHandler(webhook.Options{
		Secret:           secret,
		Logger:           logger,
		Store:            store,
		MetricsCollector: metricsCollector,
	})

	forwarder := webhook.NewWebhookForwarder(webhook.WebhookForwarderOptions{
		TargetURL:        "unix://" + socketPath,
		Storage:          store,
		MetricsCollector: metricsCollector,
		Logger:           logger,
	})

	// Start the forwarder
	go forwarder.StartForwarder(ctx)

	// Create test server with webhook handler
	server := httptest.NewServer(handler)
	defer server.Close()

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

	// Wait a bit for the event to be stored
	time.Sleep(100 * time.Millisecond)

	// Get the event from storage to verify it was stored
	events, _, err := store.ListEvents(ctx, storage.QueryOptions{})
	require.NoError(t, err)
	require.Len(t, events, 1)

	// Verify forwarded_at is null before processing
	event, err := store.GetEvent(ctx, events[0].ID)
	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Nil(t, event.ForwardedAt, "forwarded_at should be null before processing")

	// Trigger event processing
	err = forwarder.ProcessEvents(ctx)
	require.NoError(t, err)

	// Wait a bit for processing to complete
	time.Sleep(100 * time.Millisecond)

	// Verify forwarded_at is set after processing
	event, err = store.GetEvent(ctx, events[0].ID)
	require.NoError(t, err)
	require.NotNil(t, event)
	assert.NotNil(t, event.ForwardedAt, "forwarded_at should be set after processing")

	// Wait for forwarded request
	select {
	case receivedData := <-receivedCh:
		assert.Equal(t, payload, receivedData)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for forwarded request")
	}
}
