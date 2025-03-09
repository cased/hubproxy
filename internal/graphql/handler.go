package graphql

import (
	"log/slog"
	"net/http"

	"hubproxy/internal/storage"

	"github.com/graphql-go/handler"
)

// NewHandler creates a new GraphQL HTTP handler
func NewHandler(store storage.Storage, logger *slog.Logger) (http.Handler, error) {
	schema, err := NewSchema(store, logger)
	if err != nil {
		return nil, err
	}

	// Create a GraphQL HTTP handler
	h := handler.New(&handler.Config{
		Schema:     &schema.schema,
		Pretty:     true,
		GraphiQL:   true, // Enable GraphiQL interface for easy testing
		Playground: true, // Enable Playground interface as an alternative to GraphiQL
	})

	return h, nil
}
