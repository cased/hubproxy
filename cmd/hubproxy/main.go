package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"hubproxy/internal/api"
	"hubproxy/internal/graphql"
	"hubproxy/internal/metrics"
	"hubproxy/internal/security"
	"hubproxy/internal/storage"
	"hubproxy/internal/storage/sql"
	"hubproxy/internal/webhook"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
	"tailscale.com/tsnet"
)

var configFile string

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "HubProxy - A robust GitHub webhook proxy",
		Long: `HubProxy is a robust webhook proxy to enhance the reliability and security
of GitHub webhook integrations. It acts as an intermediary between GitHub
and your target services.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			viper.AutomaticEnv()
			viper.SetEnvPrefix("hubproxy")
			viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

			// Support unprefixed Tailscale env var conventions
			if os.Getenv("TS_HOSTNAME") != "" {
				viper.SetDefault("ts-hostname", os.Getenv("TS_HOSTNAME"))
			}
			if os.Getenv("TS_AUTHKEY") != "" {
				viper.SetDefault("ts-authkey", os.Getenv("TS_AUTHKEY"))
			}

			// Handle any file: prefixed values
			viperReadFile("ts-authkey")
			viperReadFile("webhook-secret")

			if err := viper.BindPFlags(cmd.Flags()); err != nil {
				return fmt.Errorf("failed to bind flags: %w", err)
			}

			// Use HUBPROXY_CONFIG if no --config is given
			if configFile == "" && os.Getenv("HUBPROXY_CONFIG") != "" {
				configFile = os.Getenv("HUBPROXY_CONFIG")
			}

			// Load config file if specified
			if configFile != "" {
				viper.SetConfigFile(configFile)
				if err := viper.ReadInConfig(); err != nil {
					return fmt.Errorf("failed to load config file: %w", err)
				}
			}

			// Skip server startup in test mode
			if viper.GetBool("test-mode") {
				return nil
			}

			return run()
		},
	}

	// Add config file flag
	cmd.Flags().StringVar(&configFile, "config", "", "Path to config file (optional)")

	// Add other flags
	flags := cmd.Flags()
	flags.String("webhook-addr", ":8080", "Public address to listen for webhooks on")
	flags.String("api-addr", ":8081", "Private address for API requests")
	flags.String("webhook-secret", "", "GitHub webhook secret (required)")
	flags.String("target-url", "", "Target URL to forward webhooks to")
	flags.String("log-level", "info", "Log level (debug, info, warn, error)")
	flags.Bool("validate-ip", true, "Validate that requests come from GitHub IPs")
	flags.Bool("trusted-proxy", false, "Trust the X-Forwarded-For header for IP validation")
	flags.Bool("enable-tailscale", false, "Enable Tailscale integration")
	flags.String("ts-authkey", "", "Tailscale auth key for tsnet")
	flags.String("ts-hostname", "hubproxy", "Tailscale hostname (will be <hostname>.<tailnet>.ts.net)")
	flags.String("db", "sqlite::memory:", "Database URI (e.g., sqlite:hubproxy.db, mysql://user:pass@host/db, postgres://user:pass@host/db)")
	flags.Duration("metrics-interval", 0*time.Minute, "Interval at which to gather database metrics")
	flags.Bool("test-mode", false, "Skip server startup for testing")

	return cmd
}

func viperReadFile(key string) {
	const filePrefix = "file:"
	value := viper.GetString(key)
	if strings.HasPrefix(value, filePrefix) {
		path := strings.TrimPrefix(value, filePrefix)
		content, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("failed to read file, using value as literal string",
				"key", key,
				"path", path,
				"error", err,
			)
			return
		}
		slog.Debug("read config from file", "key", key, "path", path)
		viper.Set(key, strings.TrimSpace(string(content)))
	}
}

func run() error {
	ctx := context.Background()

	// Setup logger
	var level slog.Level
	switch viper.GetString("log-level") {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return fmt.Errorf("invalid log level: %s", viper.GetString("log-level"))
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	// Get webhook secret from environment
	secret := viper.GetString("webhook-secret")
	if secret == "" {
		return fmt.Errorf("webhook secret is required (set HUBPROXY_WEBHOOK_SECRET environment variable)")
	}
	logger.Info("using webhook secret from environment", "secret", secret)

	// Get target URL if provided
	targetURL := viper.GetString("target-url")
	if targetURL != "" {
		// Parse target URL
		parsedURL, err := url.Parse(targetURL)
		if err != nil {
			return fmt.Errorf("invalid target URL: %w", err)
		}
		targetURL = parsedURL.String()
		logger.Info("forwarding webhooks to target URL", "url", targetURL)
	} else {
		logger.Info("running in log-only mode (no target URL specified)")
	}

	webhookHTTPClient := &http.Client{}

	// Setup optional Tailscale server
	var tsnetServer *tsnet.Server
	if viper.GetBool("enable-tailscale") {
		tsnetServer = &tsnet.Server{
			Hostname: viper.GetString("ts-hostname"),
			AuthKey:  viper.GetString("ts-authkey"),
			Logf: func(format string, args ...any) {
				logger.Debug("tsnet: " + fmt.Sprintf(format, args...))
			},
			UserLogf: func(format string, args ...any) {
				logger.Info("tsnet: " + fmt.Sprintf(format, args...))
			},
		}
		defer tsnetServer.Close()

		if err := tsnetServer.Start(); err != nil {
			return fmt.Errorf("failed to start Tailscale server: %w", err)
		}

		webhookHTTPClient = tsnetServer.HTTPClient()
	}

	// Initialize storage
	store, err := sql.New(viper.GetString("db"))
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer store.Close()

	err = store.CreateSchema(ctx)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	metricsCollector := storage.NewDBMetricsCollector(store, logger)
	metricsCollector.StartMetricsCollection(ctx, viper.GetDuration("metrics-interval"))

	// Create webhook handler
	webhookHandler := webhook.NewHandler(webhook.Options{
		Secret:           viper.GetString("webhook-secret"),
		Logger:           logger,
		Store:            store,
		ValidateIP:       viper.GetBool("validate-ip"),
		MetricsCollector: metricsCollector,
	})

	// Forwarder requires target URL be set
	if targetURL != "" {
		webhookForwarder := webhook.NewWebhookForwarder(webhook.WebhookForwarderOptions{
			TargetURL:        targetURL,
			HTTPClient:       webhookHTTPClient,
			Storage:          store,
			MetricsCollector: metricsCollector,
			Logger:           logger,
		})
		go webhookForwarder.StartForwarder(ctx)
	}

	// Create webhook server
	var webhookLn net.Listener
	webhookRouter := chi.NewRouter()

	webhookRouter.Use(metrics.Middleware)
	webhookRouter.Use(middleware.RequestID)
	if tsnetServer != nil {
		webhookRouter.Use(security.TailscaleFunnelIP(logger))
	}
	if viper.GetBool("trusted-proxy") {
		webhookRouter.Use(middleware.RealIP)
	}
	webhookRouter.Use(middleware.Logger)
	webhookRouter.Use(middleware.Heartbeat("/healthz"))
	webhookRouter.Use(middleware.Recoverer)

	webhookRouter.Handle("/webhook", webhookHandler)
	webhookSrv := &http.Server{
		Handler:      webhookRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			// Store connection reference in context so requests can access it
			return context.WithValue(ctx, security.ConnectionContextKey, c)
		},
	}

	// Create API server
	var apiLn net.Listener
	apiHandler := api.NewHandler(store, logger)
	apiRouter := chi.NewRouter()

	// Create GraphQL handler
	graphqlHandler, err := graphql.NewHandler(store, logger)
	if err != nil {
		return fmt.Errorf("failed to create GraphQL handler: %w", err)
	}

	apiRouter.Use(metrics.Middleware)
	apiRouter.Use(middleware.RequestID)
	if viper.GetBool("trusted-proxy") {
		apiRouter.Use(middleware.RealIP)
	}
	apiRouter.Use(middleware.Logger)
	apiRouter.Use(middleware.Heartbeat("/healthz"))
	apiRouter.Use(middleware.Recoverer)

	apiRouter.Get("/api/events", apiHandler.ListEvents)
	apiRouter.Get("/api/stats", apiHandler.GetStats)
	apiRouter.Get("/api/events/{id}", apiHandler.ReplayEvent)
	apiRouter.Get("/api/replay", apiHandler.ReplayRange)
	apiRouter.Handle("/metrics", promhttp.Handler())

	// Add GraphQL endpoint
	apiRouter.Handle("/graphql", graphqlHandler)

	apiSrv := &http.Server{
		Handler:      apiRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	if tsnetServer != nil {
		var err error

		apiLn, err = tsnetServer.ListenTLS("tcp", ":443")
		if err != nil {
			return fmt.Errorf("failed to listen: %w", err)
		}

		webhookLn, err = tsnetServer.ListenFunnel("tcp", ":443", tsnet.FunnelOnly())
		if err != nil {
			return fmt.Errorf("failed to listen: %w", err)
		}

		domains := tsnetServer.CertDomains()
		addr := "https://[unknown]"
		if len(domains) > 0 {
			addr = fmt.Sprintf("https://%s", domains[0])
		}
		logger.Info("Started Tailscale server", "addr", addr)
	} else {
		var err error

		webhookLn, err = net.Listen("tcp", viper.GetString("webhook-addr"))
		if err != nil {
			return fmt.Errorf("failed to listen: %w", err)
		}

		apiLn, err = net.Listen("tcp", viper.GetString("api-addr"))
		if err != nil {
			return fmt.Errorf("failed to listen: %w", err)
		}

		logger.Info("Started webhook HTTP server", "addr", webhookLn.Addr())
		logger.Info("Started API HTTP server", "addr", apiLn.Addr())
	}

	g := new(errgroup.Group)
	g.Go(func() error { return webhookSrv.Serve(webhookLn) })
	g.Go(func() error { return apiSrv.Serve(apiLn) })
	return g.Wait()
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
