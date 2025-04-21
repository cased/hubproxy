package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"hubproxy/internal/storage"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	webhookForwardedEvents = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "hubproxy_webhook_forwarded_events_total",
			Help: "Total number of webhook events forwarded to the target",
		},
	)

	webhookForwardingErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "hubproxy_webhook_forwarding_errors_total",
			Help: "Total number of webhook forwarding errors",
		},
	)
)

type WebhookForwarder struct {
	storage          storage.Storage
	metricsCollector *storage.DBMetricsCollector
	httpClient       *http.Client
	targetURL        string
	logger           *slog.Logger
	queue            chan struct{}
}

type WebhookForwarderOptions struct {
	Storage          storage.Storage
	MetricsCollector *storage.DBMetricsCollector
	HTTPClient       *http.Client
	TargetURL        string
	Logger           *slog.Logger
}

func NewWebhookForwarder(opts WebhookForwarderOptions) *WebhookForwarder {
	if opts.TargetURL == "" {
		panic("target URL is required")
	}
	if opts.Storage == nil {
		panic("storage is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	httpClient := opts.HTTPClient

	// Swap out HTTP client to use Unix socket
	if strings.HasPrefix(opts.TargetURL, "unix://") {
		socketPath := strings.TrimPrefix(opts.TargetURL, "unix://")
		httpClient = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		}
	}

	// Use default HTTP client if not provided
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	return &WebhookForwarder{
		targetURL:        opts.TargetURL,
		httpClient:       httpClient,
		storage:          opts.Storage,
		metricsCollector: opts.MetricsCollector,
		logger:           opts.Logger,
		queue:            make(chan struct{}, 1), // Buffer size 1 to allow one pending job
	}
}

// TargetURL returns the configured target URL
func (f *WebhookForwarder) TargetURL() string {
	return f.targetURL
}

func (f *WebhookForwarder) forwardEvent(ctx context.Context, event *storage.Event) {
	var targetURL string
	// http.NewRequest still needs a valid http URI, make a fake one for unix socket path
	if strings.HasPrefix(f.targetURL, "unix://") {
		targetURL = "http://127.0.0.1/webhook"
	} else {
		targetURL = f.targetURL
	}

	req, err := http.NewRequest(http.MethodPost, targetURL, strings.NewReader(string(event.Payload)))
	if err != nil {
		webhookForwardingErrors.Inc()
		f.logger.Error("failed to create request", "targetURL", targetURL, "error", err)
		return
	}

	var headers map[string][]string
	err = json.Unmarshal(event.Headers, &headers)
	if err != nil {
		webhookForwardingErrors.Inc()
		f.logger.Error("failed to parse headers", "error", err)
		return
	}

	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	if req.Header.Get("Content-Type") != "application/json" {
		f.logger.Warn("Content-Type header is not application/json", "Content-Type", req.Header.Get("Content-Type"))
	}
	if req.Header.Get("X-Github-Event") == "" {
		f.logger.Warn("X-Github-Event header is not set", "X-Github-Event", req.Header.Get("X-Github-Event"))
	}
	if req.Header.Get("X-Github-Delivery") == "" {
		f.logger.Warn("X-Github-Delivery header is not set", "X-Github-Delivery", req.Header.Get("X-Github-Delivery"))
	}
	if req.Header.Get("X-Hub-Signature-256") == "" {
		f.logger.Warn("X-Hub-Signature-256 header is not set", "X-Hub-Signature-256", req.Header.Get("X-Hub-Signature-256"))
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		webhookForwardingErrors.Inc()
		f.logger.Error("failed to forward request", "targetURL", targetURL, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		webhookForwardingErrors.Inc()
		f.logger.Error("target returned error", "status", resp.Status, "targetURL", targetURL)
		return
	}

	webhookForwardedEvents.Inc()

	err = f.storage.MarkForwarded(ctx, event.ID)
	if err != nil {
		f.logger.Error("error marking event as forwarded", "error", err)
	}
}

func (f *WebhookForwarder) ProcessEvents(ctx context.Context) error {
	// Don't ever create a WebhookForwarder if there's no target URL
	if f.targetURL == "" {
		panic("target URL is not set")
	}

	f.logger.Debug("processing webhook events from database")

	events, _, err := f.storage.ListEvents(ctx, storage.QueryOptions{OnlyNonForwarded: true})
	if err != nil {
		return fmt.Errorf("listing events: %w", err)
	}

	if len(events) == 0 {
		f.logger.Debug("no events to forward")
		return nil
	}

	f.logger.Info("forwarding webhook events", "count", len(events))

	for _, event := range events {
		f.forwardEvent(ctx, event)
	}

	f.metricsCollector.EnqueueGatherMetrics(ctx)

	return nil
}

func (f *WebhookForwarder) EnqueueProcessEvents() {
	select {
	case f.queue <- struct{}{}:
		f.logger.Debug("enqueued webhook processing job")
	default:
		f.logger.Debug("webhook processing job already pending")
	}
}

func (f *WebhookForwarder) StartForwarder(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				f.logger.Debug("stopped webhook forwarder")
				return
			case <-f.queue:
				if err := f.ProcessEvents(ctx); err != nil {
					f.logger.Error("failed to process webhook events", "error", err)
				}
			}
		}
	}()

	f.EnqueueProcessEvents()
}
