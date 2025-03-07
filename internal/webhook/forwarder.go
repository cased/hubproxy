package webhook

import (
	"context"
	"fmt"
	"log/slog"
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
	targetURL  string
	httpClient *http.Client
	storage    storage.Storage
	logger     *slog.Logger
	queue      chan struct{}
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

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", event.Type)
	req.Header.Set("X-GitHub-Delivery", event.ID)
	// TODO: Where is X-Hub-Signature-256 stored in the db?
	// req.Header.Set("X-Hub-Signature-256", event.Signature)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		webhookForwardingErrors.Inc()
		f.logger.Error("failed to forward request", "targetURL", targetURL, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		webhookForwardingErrors.Inc()
		f.logger.Error("target returned error", "status", "targetURL", targetURL, resp.Status)
		return
	}

	webhookForwardedEvents.Inc()

	// TODO: Mark event as forwarded

	if err := f.storage.StoreEvent(ctx, event); err != nil {
		f.logger.Error("failed to store event", "error", err)
		return
	}
}

func (f *WebhookForwarder) ProcessEvents(ctx context.Context) error {
	// Don't ever create a WebhookForwarder if there's no target URL
	if f.targetURL == "" {
		panic("target URL is not set")
	}

	f.logger.Debug("processing webhook events from database")

	// TODO: Only select events WEHRE forwarded_at IS NULL
	events, _, err := f.storage.ListEvents(ctx, storage.QueryOptions{})
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
