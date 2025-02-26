package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hubproxy_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		},
		[]string{"method", "path", "status", "handler"},
	)

	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hubproxy_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status", "handler"},
	)
)

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		routePattern := "unknown"
		handlerName := "unknown"

		routeCtx := chi.RouteContext(r.Context())
		if routeCtx != nil {
			routePattern = routeCtx.RoutePattern()
			if routePattern != "" {
				handlerName = routePattern
			}
		}

		next.ServeHTTP(ww, r)

		duration := time.Since(start).Seconds()
		statusCode := strconv.Itoa(ww.Status())

		RequestDuration.WithLabelValues(
			r.Method,
			routePattern,
			statusCode,
			handlerName,
		).Observe(duration)

		RequestsTotal.WithLabelValues(
			r.Method,
			routePattern,
			statusCode,
			handlerName,
		).Inc()
	})
}
