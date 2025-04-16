package graphql

import (
	"log/slog"

	"hubproxy/internal/storage"

	"github.com/graphql-go/graphql"
)

// Schema defines the GraphQL schema and resolvers
type Schema struct {
	schema graphql.Schema
	store  storage.Storage
	logger *slog.Logger
}

// NewSchema creates a new GraphQL schema with the given storage
func NewSchema(store storage.Storage, logger *slog.Logger) (*Schema, error) {
	s := &Schema{
		store:  store,
		logger: logger,
	}

	// Define Event type
	eventType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Event",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.String,
			},
			"type": &graphql.Field{
				Type: graphql.String,
			},
			"headers": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if event, ok := p.Source.(*storage.Event); ok {
						return string(event.Headers), nil
					}
					return nil, nil
				},
			},
			"payload": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if event, ok := p.Source.(*storage.Event); ok {
						return string(event.Payload), nil
					}
					return nil, nil
				},
			},
			"createdAt": &graphql.Field{
				Type: graphql.DateTime,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if event, ok := p.Source.(*storage.Event); ok {
						return event.CreatedAt, nil
					}
					return nil, nil
				},
			},
			"status": &graphql.Field{
				Type: graphql.String,
			},
			"error": &graphql.Field{
				Type: graphql.String,
			},
			"repository": &graphql.Field{
				Type: graphql.String,
			},
			"sender": &graphql.Field{
				Type: graphql.String,
			},
			"replayedFrom": &graphql.Field{
				Type: graphql.String,
			},
			"originalTime": &graphql.Field{
				Type: graphql.DateTime,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if event, ok := p.Source.(*storage.Event); ok {
						return event.OriginalTime, nil
					}
					return nil, nil
				},
			},
		},
	})

	// Define EventsResponse type
	eventsResponseType := graphql.NewObject(graphql.ObjectConfig{
		Name: "EventsResponse",
		Fields: graphql.Fields{
			"events": &graphql.Field{
				Type: graphql.NewList(eventType),
			},
			"total": &graphql.Field{
				Type: graphql.Int,
			},
		},
	})

	// Define ReplayResponse type
	replayResponseType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ReplayResponse",
		Fields: graphql.Fields{
			"replayedCount": &graphql.Field{
				Type: graphql.Int,
			},
			"events": &graphql.Field{
				Type: graphql.NewList(eventType),
			},
		},
	})

	// Define Stats type
	statType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Stat",
		Fields: graphql.Fields{
			"type": &graphql.Field{
				Type: graphql.String,
			},
			"count": &graphql.Field{
				Type: graphql.Int,
			},
		},
	})

	// Define root query
	rootQuery := graphql.NewObject(graphql.ObjectConfig{
		Name: "RootQuery",
		Fields: graphql.Fields{
			"events": &graphql.Field{
				Type: eventsResponseType,
				Args: graphql.FieldConfigArgument{
					"type": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"repository": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"sender": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"status": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"since": &graphql.ArgumentConfig{
						Type: graphql.DateTime,
					},
					"until": &graphql.ArgumentConfig{
						Type: graphql.DateTime,
					},
					"limit": &graphql.ArgumentConfig{
						Type: graphql.Int,
					},
					"offset": &graphql.ArgumentConfig{
						Type: graphql.Int,
					},
				},
				Resolve: s.resolveEvents,
			},
			"event": &graphql.Field{
				Type: eventType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.String),
					},
				},
				Resolve: s.resolveEvent,
			},
			"stats": &graphql.Field{
				Type: graphql.NewList(statType),
				Args: graphql.FieldConfigArgument{
					"since": &graphql.ArgumentConfig{
						Type: graphql.DateTime,
					},
				},
				Resolve: s.resolveStats,
			},
		},
	})

	// Define root mutation
	rootMutation := graphql.NewObject(graphql.ObjectConfig{
		Name: "RootMutation",
		Fields: graphql.Fields{
			"replayEvent": &graphql.Field{
				Type: replayResponseType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.String),
					},
				},
				Resolve: s.resolveReplayEvent,
			},
			"replayRange": &graphql.Field{
				Type: replayResponseType,
				Args: graphql.FieldConfigArgument{
					"since": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.DateTime),
					},
					"until": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.DateTime),
					},
					"type": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"repository": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"sender": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"limit": &graphql.ArgumentConfig{
						Type: graphql.Int,
					},
				},
				Resolve: s.resolveReplayRange,
			},
		},
	})

	// Create schema
	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query:    rootQuery,
		Mutation: rootMutation,
	})
	if err != nil {
		return nil, err
	}

	s.schema = schema
	return s, nil
}
