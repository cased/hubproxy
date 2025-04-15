package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"hubproxy/internal/storage"
	"hubproxy/internal/testutil"

	"github.com/graphql-go/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphQLQueries(t *testing.T) {
	// Setup test environment
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	store := testutil.SetupTestDB(t)

	// Add test data
	setupTestData(t, store)

	schema, err := NewSchema(store, logger)
	require.NoError(t, err)

	// Test cases
	testCases := []struct {
		name     string
		query    string
		validate func(t *testing.T, result *graphql.Result)
	}{
		{
			name: "Query Events",
			query: `
				query {
					events {
						events {
							id
							type
							repository
							sender
						}
						total
					}
				}
			`,
			validate: func(t *testing.T, result *graphql.Result) {
				assert.Nil(t, result.Errors, "GraphQL query returned errors")

				data := result.Data.(map[string]interface{})
				eventsData := data["events"].(map[string]interface{})
				events := eventsData["events"].([]interface{})

				// Convert total to int for consistent comparison
				var total int
				switch v := eventsData["total"].(type) {
				case float64:
					total = int(v)
				case int:
					total = v
				default:
					t.Fatalf("Unexpected type for total: %T", eventsData["total"])
				}

				// Verify total count
				assert.Equal(t, 2, total)

				// Verify we have 2 events
				assert.Len(t, events, 2)

				// Sort events by ID to ensure consistent ordering for validation
				sort.Slice(events, func(i, j int) bool {
					return events[i].(map[string]interface{})["id"].(string) <
						events[j].(map[string]interface{})["id"].(string)
				})

				// Validate first event
				event1 := events[0].(map[string]interface{})
				assert.Equal(t, "test-event-1", event1["id"])
				assert.Equal(t, "push", event1["type"])
				assert.Equal(t, "test-repo/test", event1["repository"])
				assert.Equal(t, "test-user", event1["sender"])

				// Validate second event
				event2 := events[1].(map[string]interface{})
				assert.Equal(t, "test-event-2", event2["id"])
				assert.Equal(t, "pull_request", event2["type"])
				assert.Equal(t, "test-repo/test", event2["repository"])
				assert.Equal(t, "test-user", event2["sender"])
			},
		},
		{
			name: "Query Single Event",
			query: `
				query {
					event(id: "test-event-1") {
						id
						type
						repository
						sender
					}
				}
			`,
			validate: func(t *testing.T, result *graphql.Result) {
				assert.Nil(t, result.Errors, "GraphQL query returned errors")

				data := result.Data.(map[string]interface{})
				event := data["event"].(map[string]interface{})

				assert.Equal(t, "test-event-1", event["id"])
				assert.Equal(t, "push", event["type"])
				assert.Equal(t, "test-repo/test", event["repository"])
				assert.Equal(t, "test-user", event["sender"])
			},
		},
		{
			name: "Query Stats",
			query: `
				query {
					stats {
						type
						count
					}
				}
			`,
			validate: func(t *testing.T, result *graphql.Result) {
				assert.Nil(t, result.Errors, "GraphQL query returned errors")

				data := result.Data.(map[string]interface{})
				stats := data["stats"].([]interface{})

				// Verify we have 2 stat entries
				assert.Len(t, stats, 2)

				// Create a map of type to count for easier validation
				statMap := make(map[string]int)
				for _, stat := range stats {
					s := stat.(map[string]interface{})
					statType := s["type"].(string)

					// Handle different numeric types
					var count int
					switch v := s["count"].(type) {
					case float64:
						count = int(v)
					case int:
						count = v
					default:
						t.Fatalf("Unexpected type for count: %T", s["count"])
					}

					statMap[statType] = count
				}

				// Validate counts
				assert.Equal(t, 1, statMap["push"])
				assert.Equal(t, 1, statMap["pull_request"])
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := executeQuery(schema.schema, tc.query, nil)
			tc.validate(t, result)
		})
	}
}

func TestGraphQLMutations(t *testing.T) {
	// Setup test environment
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	store := testutil.SetupTestDB(t)

	// Add test data
	setupTestData(t, store)

	schema, err := NewSchema(store, logger)
	require.NoError(t, err)

	// Test cases for mutations
	t.Run("Replay Event", func(t *testing.T) {
		query := `
			mutation {
				replayEvent(id: "test-event-1") {
					replayedCount
					events {
						id
						type
						status
						replayedFrom
					}
				}
			}
		`
		result := executeQuery(schema.schema, query, nil)
		assert.Nil(t, result.Errors, "GraphQL mutation returned errors")

		data := result.Data.(map[string]interface{})
		replayEvent := data["replayEvent"].(map[string]interface{})

		// Handle different numeric types for replayedCount
		var replayedCount int
		switch v := replayEvent["replayedCount"].(type) {
		case float64:
			replayedCount = int(v)
		case int:
			replayedCount = v
		default:
			t.Fatalf("Unexpected type for replayedCount: %T", replayEvent["replayedCount"])
		}

		// Verify count
		assert.Equal(t, 1, replayedCount)

		// Check the events
		events := replayEvent["events"].([]interface{})
		require.Len(t, events, 1, "Expected 1 replayed event")

		event := events[0].(map[string]interface{})
		assert.True(t, strings.HasPrefix(event["id"].(string), "test-event-1-replay-"),
			"Replayed event ID should start with 'test-event-1-replay-'")
		assert.Equal(t, "push", event["type"])
		assert.Equal(t, "replayed", event["status"])
		assert.Equal(t, "test-event-1", event["replayedFrom"])
	})
}

func TestGraphQLHandler(t *testing.T) {
	// Setup test environment
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	store := testutil.SetupTestDB(t)

	// Add test data
	setupTestData(t, store)

	// Create handler
	handler, err := NewHandler(store, logger)
	require.NoError(t, err)

	// Create test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Test query
	query := `{"query": "{ events { total events { id type } } }"}`
	resp, err := http.Post(server.URL, "application/json", bytes.NewBufferString(query))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check response
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// Verify data exists and has no errors
	assert.Contains(t, result, "data")
	assert.NotContains(t, result, "errors")

	// Verify events data
	data := result["data"].(map[string]interface{})
	events := data["events"].(map[string]interface{})
	assert.Equal(t, float64(2), events["total"])
	assert.Len(t, events["events"], 2)
}

// Helper function to execute GraphQL queries
func executeQuery(schema graphql.Schema, query string, variables map[string]interface{}) *graphql.Result {
	result := graphql.Do(graphql.Params{
		Schema:         schema,
		RequestString:  query,
		VariableValues: variables,
		Context:        context.Background(),
	})
	return result
}

// Helper function to set up test data
func setupTestData(t *testing.T, store storage.Storage) {
	// Add test events
	now := time.Now()

	event1 := &storage.Event{
		ID:         "test-event-1",
		Type:       "push",
		Payload:    []byte(`{"ref": "refs/heads/main"}`),
		CreatedAt:  now.Add(-1 * time.Hour),
		Status:     "received",
		Repository: "test-repo/test",
		Sender:     "test-user",
	}

	event2 := &storage.Event{
		ID:         "test-event-2",
		Type:       "pull_request",
		Payload:    []byte(`{"action": "opened"}`),
		CreatedAt:  now,
		Status:     "received",
		Repository: "test-repo/test",
		Sender:     "test-user",
	}

	err := store.StoreEvent(context.Background(), event1)
	require.NoError(t, err)

	err = store.StoreEvent(context.Background(), event2)
	require.NoError(t, err)
}
