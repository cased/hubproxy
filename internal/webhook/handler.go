package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"hubproxy/internal/security"
	"hubproxy/internal/storage"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	webhookSignatureErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "hubproxy_webhook_signature_errors_total",
			Help: "Total number of webhook signature verification errors",
		},
	)

	webhookStoredEvents = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "hubproxy_webhook_stored_events_total",
			Help: "Total number of webhook events stored",
		},
	)

	webhookForwardedRequests = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "hubproxy_webhook_forwarded_requests_total",
			Help: "Total number of webhook events forwarded",
		},
	)
	webhookForwardedErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "hubproxy_webhook_forwarded_errors_total",
			Help: "Total number of webhook forwarding errors",
		},
	)

	webhookBlockedIPs = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "hubproxy_webhook_blocked_ips_total",
			Help: "Total number of webhook requests blocked from non-GitHub IPs",
		},
	)
)

type Handler struct {
	secret      string
	targetURL   string
	httpClient  *http.Client
	logger      *slog.Logger
	ipValidator *security.IPValidator
	store       storage.Storage
}

type Options struct {
	Secret     string
	TargetURL  string
	Logger     *slog.Logger
	ValidateIP bool
	Store      storage.Storage
}

func NewHandler(opts Options) *Handler {
	var ipValidator *security.IPValidator
	if opts.ValidateIP {
		ipValidator = security.NewIPValidator(1*time.Hour, false) // Update IP ranges every hour
	}

	return &Handler{
		secret:      opts.Secret,
		targetURL:   opts.TargetURL,
		httpClient:  &http.Client{},
		logger:      opts.Logger,
		ipValidator: ipValidator,
		store:       opts.Store,
	}
}

// VerifySignature verifies the GitHub webhook signature
// Format: sha256=<hex-digest>
func (h *Handler) VerifySignature(header http.Header, payload []byte) error {
	signature := header.Get("X-Hub-Signature-256")
	if signature == "" {
		h.logger.Error("missing signature")
		return fmt.Errorf("missing signature")
	}

	h.logger.Debug("verifying signature",
		"header", signature,
		"payload_length", len(payload),
		"secret_length", len(h.secret))

	if !strings.HasPrefix(signature, "sha256=") {
		h.logger.Error("invalid signature format")
		return fmt.Errorf("invalid signature format")
	}

	providedSignature := strings.TrimPrefix(signature, "sha256=")

	// Decode hex signature
	providedBytes, err := hex.DecodeString(providedSignature)
	if err != nil {
		h.logger.Error("invalid signature hex", "error", err)
		return fmt.Errorf("invalid signature hex: %v", err)
	}

	// Calculate expected signature
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(payload)
	expectedBytes := mac.Sum(nil)
	expectedSignature := hex.EncodeToString(expectedBytes)

	h.logger.Debug("comparing signatures",
		"provided", providedSignature,
		"expected", expectedSignature,
		"secret", h.secret,
		"payload", string(payload))

	if !hmac.Equal(providedBytes, expectedBytes) {
		h.logger.Error("invalid signature",
			"provided", providedSignature,
			"expected", expectedSignature)
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// ValidateGitHubEvent validates required GitHub webhook headers
func (h *Handler) ValidateGitHubEvent(r *http.Request) error {
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		return fmt.Errorf("missing event type")
	}

	// Validate IP if enabled
	if h.ipValidator != nil {
		ip := strings.Split(r.RemoteAddr, ":")[0]
		if !h.ipValidator.IsGitHubIP(ip) {
			h.logger.Error("request from non-GitHub IP", "ip", ip)
			return fmt.Errorf("request from non-GitHub IP: %s", ip)
		}
	}

	return nil
}

// TargetURL returns the configured target URL
func (h *Handler) TargetURL() string {
	return h.targetURL
}

func (h *Handler) Forward(payload []byte, headers http.Header) error {
	req, err := http.NewRequest(http.MethodPost, h.targetURL, strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Forward relevant headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", headers.Get("X-GitHub-Event"))
	req.Header.Set("X-GitHub-Delivery", headers.Get("X-GitHub-Delivery"))
	req.Header.Set("X-Hub-Signature-256", headers.Get("X-Hub-Signature-256"))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("forwarding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("target returned error: %s", resp.Status)
	}

	return nil
}

// ServeHTTP handles incoming webhook requests
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.logger.Error("validation error", "error", fmt.Sprintf("invalid method: %s", r.Method))
		http.Error(w, fmt.Sprintf("invalid method: %s", r.Method), http.StatusMethodNotAllowed)
		return
	}

	if err := h.ValidateGitHubEvent(r); err != nil {
		h.logger.Error("validation error", "error", err)
		webhookBlockedIPs.Inc()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("error reading body", "error", err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	if err := h.VerifySignature(r.Header, payload); err != nil {
		h.logger.Error("signature verification error", "error", err)
		webhookSignatureErrors.Inc()
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Store the webhook event
	if h.store != nil {
		event := &storage.Event{
			ID:         r.Header.Get("X-GitHub-Delivery"), // Use GitHub's delivery ID
			Type:       r.Header.Get("X-GitHub-Event"),
			Payload:    json.RawMessage(payload),
			CreatedAt:  time.Now(),
			Status:     "received",
			Repository: "", // Extract from payload if needed
			Sender:     "", // Extract from payload if needed
		}

		// Extract repository and sender from payload
		var payloadMap map[string]interface{}
		if err := json.Unmarshal(payload, &payloadMap); err == nil {
			if repo, ok := payloadMap["repository"].(map[string]interface{}); ok {
				if fullName, ok := repo["full_name"].(string); ok {
					event.Repository = fullName
				}
			}
			if sender, ok := payloadMap["sender"].(map[string]interface{}); ok {
				if login, ok := sender["login"].(string); ok {
					event.Sender = login
				}
			}
		}

		if err := h.store.StoreEvent(r.Context(), event); err != nil {
			h.logger.Error("error storing event", "error", err)
			// Continue even if storage fails
		} else {
			webhookStoredEvents.Inc()
		}
	}

	if h.targetURL != "" {
		if err := h.Forward(payload, r.Header); err != nil {
			h.logger.Error("forwarding error", "error", err)
			webhookForwardedErrors.Inc()
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		} else {
			webhookForwardedRequests.Inc()
		}
	}

	w.WriteHeader(http.StatusOK)
}
