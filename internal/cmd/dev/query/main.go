package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"hubproxy/internal/storage"
	"hubproxy/internal/storage/factory"
)

func main() {
	var (
		limit     = flag.Int("limit", 10, "Maximum number of events to show")
		since     = flag.Duration("since", 24*time.Hour, "Show events since duration (e.g. 1h, 24h)")
		stats     = flag.Bool("stats", true, "Show event type statistics")
		dbPath    = flag.String("db", ".cache/hubproxy.db", "Path to SQLite database")
		eventType = flag.String("type", "", "Filter by event type (e.g. push, pull_request)")
		repo      = flag.String("repo", "", "Filter by repository (e.g. owner/repo)")
	)
	flag.Parse()

	var err error
	var store storage.Storage

	// Connect to SQLite database
	store, err = factory.NewStorageFromURI("sqlite:" + *dbPath)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer store.Close()

	// Get event type statistics if requested
	if *stats {
		sinceTime := time.Now().Add(-*since)
		var eventStats map[string]int64
		eventStats, err = store.GetStats(nil, sinceTime)
		if err != nil {
			log.Fatalf("Failed to get event stats: %v", err)
		}

		fmt.Printf("\nEvent Type Statistics (since %s):\n", sinceTime.Format(time.RFC3339))
		for eventType, count := range eventStats {
			fmt.Printf("  %s: %d\n", eventType, count)
		}
		fmt.Println()
	}

	// Query events
	opts := storage.QueryOptions{
		Limit:  *limit,
		Since:  time.Now().Add(-*since),
		Status: "completed",
	}
	if *eventType != "" {
		opts.Types = []string{*eventType}
	}
	if *repo != "" {
		opts.Repository = *repo
	}

	events, total, err := store.ListEvents(nil, opts)
	if err != nil {
		log.Fatalf("Failed to list events: %v", err)
	}

	fmt.Printf("Found %d events (showing %d):\n", total, len(events))
	for _, event := range events {
		fmt.Printf("  %s: %s/%s (%s) [%s]\n",
			event.Type,
			event.Repository,
			event.ID,
			event.CreatedAt.Format(time.RFC3339),
			event.Status,
		)
	}
}
