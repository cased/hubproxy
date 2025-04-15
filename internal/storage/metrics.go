package storage

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	eventCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hubproxy_db_events_count",
		Help: "Number of events stored in the database",
	}, []string{"type"})
)

type DBMetricsCollector struct {
	storage Storage
	logger  *slog.Logger
	queue   chan struct{}
}

func NewDBMetricsCollector(storage Storage, logger *slog.Logger) *DBMetricsCollector {
	return &DBMetricsCollector{
		storage: storage,
		logger:  logger,
		queue:   make(chan struct{}, 1),
	}
}

func (c *DBMetricsCollector) GatherMetrics(ctx context.Context) error {
	c.logger.Debug("gathering metrics")

	stats, err := c.storage.GetStats(ctx, time.Time{})
	if err != nil {
		return err
	}

	for eventType, count := range stats {
		eventCount.WithLabelValues(eventType).Set(float64(count))
	}

	return nil
}

func (c *DBMetricsCollector) EnqueueGatherMetrics(ctx context.Context) {
	select {
	case c.queue <- struct{}{}:
		c.logger.Debug("enqueued metrics job")
	default:
		c.logger.Debug("metrics job already pending")
	}
}

func (c *DBMetricsCollector) StartMetricsCollection(ctx context.Context, interval time.Duration) {
	if c.storage == nil {
		c.logger.Debug("storage is nil, not starting metrics collection")
		return
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				c.logger.Debug("stopped metrics collector")
				return
			case <-c.queue:
				if err := c.GatherMetrics(ctx); err != nil {
					c.logger.Error("failed to gather metrics", "error", err)
				}
			}
		}
	}()

	// interval is optional
	if interval > 0 {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			c.logger.Debug("starting periodic metrics collector", "interval", interval)

			for {
				select {
				case <-ctx.Done():
					c.logger.Debug("stopped periodic metrics collector")
					return
				case <-ticker.C:
					c.EnqueueGatherMetrics(ctx)
				}
			}
		}()
	}

	c.EnqueueGatherMetrics(ctx)
}
