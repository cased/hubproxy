package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"hubproxy/internal/api"
	"hubproxy/internal/storage"
	"hubproxy/internal/storage/factory"
	"hubproxy/internal/webhook"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

			if err := viper.BindPFlags(cmd.Flags()); err != nil {
				return fmt.Errorf("failed to bind flags: %w", err)
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
	flags.String("target-url", "", "Target URL to forward webhooks to")
	flags.String("log-level", "info", "Log level (debug, info, warn, error)")
	flags.Bool("validate-ip", true, "Validate that requests come from GitHub IPs")
	flags.String("ts-authkey", "", "Tailscale auth key for tsnet")
	flags.String("ts-hostname", "hubproxy", "Tailscale hostname (will be <hostname>.<tailnet>.ts.net)")
	flags.String("db", "", "Database URI (e.g., sqlite:hubproxy.db, mysql://user:pass@host/db, postgres://user:pass@host/db)")
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

	// Initialize storage if DB URI is provided
	var store storage.Storage
	dbURI := viper.GetString("db")
	if dbURI != "" {
		var err error
		store, err = factory.NewStorageFromURI(dbURI)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}
		defer store.Close()

		if err := store.CreateSchema(ctx); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
	}

	// Create webhook handler
	webhookHandler := webhook.NewHandler(webhook.Options{
		Secret:     viper.GetString("webhook-secret"),
		TargetURL:  targetURL,
		Logger:     logger,
		Store:      store,
		ValidateIP: viper.GetBool("validate-ip"),
	})

	// Create API handler
	apiHandler := api.NewHandler(store, logger)

	// Create router and register handlers
	mux := http.NewServeMux()
	mux.Handle("/webhook", webhookHandler)
	mux.Handle("/api/events", http.HandlerFunc(apiHandler.ListEvents))
	mux.Handle("/api/stats", http.HandlerFunc(apiHandler.GetStats))
	mux.Handle("/api/events/", http.HandlerFunc(apiHandler.ReplayEvent)) // Handle replay endpoint
	mux.Handle("/api/replay", http.HandlerFunc(apiHandler.ReplayRange))  // Handle range replay
	mux.Handle("/metrics", promhttp.Handler())                           // Add Prometheus metrics endpoint

	// Start server
	var srv *http.Server
	if authKey := viper.GetString("ts-authkey"); authKey != "" {
		// Run as Tailscale service
		hostname := viper.GetString("ts-hostname")

		s := &tsnet.Server{
			Hostname: hostname,
			AuthKey:  authKey,
		}
		defer s.Close()

		publicLn, err := s.ListenFunnel("tcp", ":443", tsnet.FunnelOnly())
		if err != nil {
			return fmt.Errorf("failed to listen: %w", err)
		}

		// TODO: Serve internal API routes on this listener
		// privateLn, err := s.ListenFunnel("tcp", ":443", tsnet.FunnelOnly())
		// if err != nil {
		// 	return fmt.Errorf("failed to listen: %w", err)
		// }

		srv = &http.Server{
			Addr:         ":443",
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		logger.Info("Started Tailscale server",
			"addr", fmt.Sprintf("https://%s", s.CertDomains()[0]),
		)
		return srv.Serve(publicLn)
	} else {
		// Run as regular HTTP server
		srv = &http.Server{
			Addr:         ":8080",
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		logger.Info("Started HTTP server", "addr", srv.Addr)
		return srv.ListenAndServe()
	}
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
