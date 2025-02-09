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
	"hubproxy/internal/storage/sql/mysql"
	"hubproxy/internal/storage/sql/postgres"
	"hubproxy/internal/storage/sql/sqlite"
	"hubproxy/internal/webhook"
	"log/slog"

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

			// Always validate target URL
			targetURL := viper.GetString("target-url")
			if targetURL == "" {
				return fmt.Errorf("target URL is required")
			}
			if _, err := url.Parse(targetURL); err != nil {
				return fmt.Errorf("invalid target URL: %w", err)
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
	flags.String("db-type", "sqlite", "Database type (sqlite, mysql, postgres)")
	flags.String("db-dsn", "hubproxy.db", "Database DSN (connection string)")
	flags.Bool("test-mode", false, "Skip server startup for testing")

	return cmd
}

func run() error {
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

	// Validate flags
	if viper.GetString("target-url") == "" {
		return fmt.Errorf("target URL is required")
	}

	// Parse target URL
	targetURL, err := url.Parse(viper.GetString("target-url"))
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}

	// Initialize storage
	var store storage.Storage
	var storageErr error
	switch viper.GetString("db-type") {
	case "sqlite":
		store, storageErr = sqlite.NewStorage(viper.GetString("db-dsn"))
	case "mysql":
		var mysqlCfg storage.Config
		mysqlCfg, storageErr = parseMySQLDSN(viper.GetString("db-dsn"))
		if storageErr != nil {
			return fmt.Errorf("invalid MySQL DSN: %w", storageErr)
		}
		store, storageErr = mysql.NewStorage(mysqlCfg)
		if storageErr != nil {
			return fmt.Errorf("failed to initialize MySQL storage: %w", storageErr)
		}
	case "postgres":
		var pgCfg storage.Config
		pgCfg, storageErr = parsePostgresDSN(viper.GetString("db-dsn"))
		if storageErr != nil {
			return fmt.Errorf("invalid Postgres DSN: %w", storageErr)
		}
		store, storageErr = postgres.NewStorage(pgCfg)
		if storageErr != nil {
			return fmt.Errorf("failed to initialize Postgres storage: %w", storageErr)
		}
	default:
		return fmt.Errorf("unsupported database type: %s", viper.GetString("db-type"))
	}
	if storageErr != nil {
		return fmt.Errorf("failed to initialize storage: %w", storageErr)
	}
	defer store.Close()

	if err := store.CreateSchema(context.Background()); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Create webhook handler
	webhookHandler := webhook.NewHandler(webhook.Options{
		Secret:     viper.GetString("webhook-secret"),
		TargetURL:  targetURL.String(),
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

	// Start server
	var srv *http.Server
	if viper.GetString("ts-authkey") != "" {
		// Run as Tailscale service
		s := &tsnet.Server{
			Hostname: viper.GetString("ts-hostname"),
			AuthKey:  viper.GetString("ts-authkey"),
		}
		defer s.Close()

		ln, err := s.ListenFunnel("tcp", ":443", tsnet.FunnelOnly())
		if err != nil {
			return fmt.Errorf("failed to listen: %w", err)
		}

		// Get our Tailscale IP
		client, err := s.LocalClient()
		if err != nil {
			return fmt.Errorf("failed to get local client: %w", err)
		}

		status, err := client.Status(context.Background())
		if err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}

		// Get our hostname from Tailscale
		hostname := status.Self.DNSName
		logger.Info("Started Tailscale server",
			"url", fmt.Sprintf("https://%s", hostname),
			"tailnet", strings.Split(hostname, ".")[1],
		)

		srv = &http.Server{
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		return srv.Serve(ln)
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

// parseMySQLDSN parses MySQL DSN into Config
// Format: user:pass@tcp(host:port)/dbname
func parseMySQLDSN(dsn string) (storage.Config, error) {
	// Extract username and password
	parts := strings.Split(dsn, "@")
	if len(parts) != 2 {
		return storage.Config{}, fmt.Errorf("invalid MySQL DSN format")
	}
	userPass := parts[0]
	credentials := strings.Split(userPass, ":")
	if len(credentials) != 2 {
		return storage.Config{}, fmt.Errorf("invalid MySQL DSN format")
	}
	username := credentials[0]
	password := credentials[1]

	// Extract host and port from tcp(host:port)
	remainder := parts[1]
	tcpParts := strings.Split(remainder, ")/")
	if len(tcpParts) != 2 {
		return storage.Config{}, fmt.Errorf("invalid MySQL DSN format")
	}

	hostPort := strings.TrimPrefix(tcpParts[0], "tcp(")
	hostPortParts := strings.Split(hostPort, ":")
	if len(hostPortParts) != 2 {
		return storage.Config{}, fmt.Errorf("invalid MySQL DSN format")
	}
	host := hostPortParts[0]
	var port int
	if _, err := fmt.Sscanf(hostPortParts[1], "%d", &port); err != nil {
		return storage.Config{}, fmt.Errorf("parsing port: %w", err)
	}

	// Extract database name
	database := strings.Split(tcpParts[1], "?")[0]

	return storage.Config{
		Host:     host,
		Port:     port,
		Database: database,
		Username: username,
		Password: password,
	}, nil
}

// parsePostgresDSN parses Postgres DSN into Config
// Format: postgres://user:pass@host:port/dbname
func parsePostgresDSN(dsn string) (storage.Config, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return storage.Config{}, fmt.Errorf("parsing PostgreSQL DSN: %w", err)
	}

	password, _ := u.User.Password()
	var port int
	if u.Port() != "" {
		if _, err := fmt.Sscanf(u.Port(), "%d", &port); err != nil {
			return storage.Config{}, fmt.Errorf("parsing port: %w", err)
		}
	} else {
		port = 5432 // default postgres port
	}

	return storage.Config{
		Host:     u.Hostname(),
		Port:     port,
		Database: strings.TrimPrefix(u.Path, "/"),
		Username: u.User.Username(),
		Password: password,
	}, nil
}
