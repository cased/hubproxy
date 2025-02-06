package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"hubproxy/internal/storage"
	"hubproxy/internal/storage/sql/sqlite"
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
	store, err = sqlite.NewStorage(*dbPath)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer store.Close()

	// Get event type statistics if requested
	if *stats {
		sinceTime := time.Now().Add(-*since)
		var eventStats map[string]int64
		var getStatsErr error

		if len(os.Args) > 1 {
			sinceTime, getStatsErr = time.Parse(time.RFC3339, os.Args[1])
			if getStatsErr != nil {
				log.Fatal(getStatsErr)
			}
		}

		eventStats, getStatsErr = store.GetStats(context.Background(), sinceTime)
		if getStatsErr != nil {
			log.Fatalf("Failed to get event type stats: %v", getStatsErr)
		}

		fmt.Printf("\nEvent Type Statistics (since %s):\n", sinceTime.Format(time.RFC3339))
		for eventType, count := range eventStats {
			fmt.Printf("  %s: %d\n", eventType, count)
		}
		return
	}

	// Query events
	var events []*storage.Event
	var total int
	sinceTime := time.Now().Add(-*since)

	// Build query options
	opts := storage.QueryOptions{
		Limit:      *limit,
		Since:      sinceTime,
		Repository: *repo,
		Offset:     0,
	}
	if *eventType != "" {
		opts.Types = []string{*eventType}
	}

	events, total, err = store.ListEvents(context.Background(), opts)
	if err != nil {
		log.Fatalf("Failed to query events: %v", err)
	}

	// Print results
	fmt.Printf("\nShowing %d of %d events since %s:\n", len(events), total, sinceTime.Format(time.RFC3339))
	fmt.Println("----------------------------------------")
	for _, event := range events {
		fmt.Printf("ID:        %s\n", event.ID)
		fmt.Printf("Type:      %s\n", event.Type)
		fmt.Printf("Repo:      %s\n", event.Repository)
		fmt.Printf("Sender:    %s\n", event.Sender)
		fmt.Printf("Status:    %s\n", event.Status)
		fmt.Printf("Timestamp: %s\n", event.CreatedAt.Format(time.RFC3339))
		if event.Error != "" {
			fmt.Printf("Error:     %s\n", event.Error)
		}
		fmt.Println("----------------------------------------")
	}
}
