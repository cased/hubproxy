# HubProxy

## Overview

HubProxy is a proxy for GitHub webhooks, built for people building with GitHub at scale. It fixes a lot of stuff, and makes life easier.

### Key Features

- **Webhook Verification**: Cryptographically verifies GitHub webhook signatures to ensure authenticity
- **Event Persistence**: Stores webhook events in a database (`SQLite`/`PostgreSQL`/`MySQL`) for audit and replay
- **Event Replay**:
  - Replay individual events by ID for testing or recovery
  - Replay events within a specific time range with filtering options
  - After a replay, you'll have two rows in the `events` table:
    1. The original event (unchanged)
    2. A new event with:
       - Same payload as original
       - `status="replayed"`
       - ID format: `original-id-replay-uuid`
       - Original event ID stored in `replayed_from`
  - Range replay has a default limit of 100 events (override with `limit`)
  - Filter range replays by `type`, `repository`, and `sender`
  - Response includes `replayed_count` and list of replayed events
  - Filter events by status using `GET /api/events?status=[received|replayed|completed|pending]`
- **Event Filtering**: 
  - Filter events by type, repository, sender, and time range
  - Query historical events through a RESTful API
  - Get event statistics and delivery metrics
- **REST API**:
  - List and search webhook events with pagination
  - View event type statistics over time
  - Replay single events or event ranges
  - Filter and query capabilities for all operations
- **Monitoring**: 
  - Provides metrics and logging for webhook delivery status and performance
  - Track event patterns and volume through API statistics

### Why HubProxy?

1. **Reliability**: 
   - Never miss a webhook due to temporary service outages or bad deploys of your application
   - Replay events after recovering from downtime
   - Queue and retry failed deliveries automatically

2. **Security**:
   - Verify webhook authenticity using GitHub's HMAC signatures
   - Centralized secret management
   - Single point of security auditing
   - Automatically verify GitHub IP origins (often missed in webhooks implementations)

3. **Observability**:
   - Track webhook delivery status and latency
   - Debug integration issues with detailed logging
   - Monitor webhook patterns and volume

4. **Development**:
   - Test new integrations against real historical events
   - Debug webhook handlers without reconfiguring GitHub
   - Simulate webhook delivery for development

### Architecture

HubProxy consists of three main components:

1. **Webhook Handler**: Receives, validates, and forwards GitHub webhooks
2. **Storage Layer**: Persists webhook events and delivery status
3. **API Server**: Provides REST endpoints for querying and replaying events

The system is designed to be horizontally scalable and can handle high webhook volumes while maintaining strict delivery guarantees.

## Development and Testing

### Prerequisites

- Go 1.22 or later
- SQLite (default), PostgreSQL 14+, or MySQL 8+ for event storage

### Database

HubProxy uses [SQLC](https://sqlc.dev/) to generate type-safe Go code from SQL queries. This provides:
- Compile-time SQL validation
- Type-safe query methods
- Zero runtime reflection
- Native support for JSON and custom types

SQLite is used by default for development, but PostgreSQL or MySQL are recommended for production:
```bash
# SQLite (default for development)
hubproxy --db sqlite:.cache/hubproxy.db

# PostgreSQL
hubproxy --db "postgres://user:password@localhost:5432/hubproxy?sslmode=disable"

# MySQL
hubproxy --db "mysql://user:password@tcp(localhost:3306)/hubproxy"
```

### Schema

The database schema is defined in `internal/storage/sql/schema.sql`. Here's a simplified version (actual types may vary by database):

```sql
CREATE TABLE events (
    id          VARCHAR(255) PRIMARY KEY,    -- GitHub delivery ID
    type        VARCHAR(50) NOT NULL,       -- GitHub event type
    payload     TEXT NOT NULL,              -- Event payload as JSON
    created_at  TIMESTAMP NOT NULL,         -- Event creation time
    status      VARCHAR(20) NOT NULL,       -- Delivery status
    error       TEXT,                       -- Error message if failed
    repository  VARCHAR(255),               -- Repository name
    sender      VARCHAR(255),               -- Event sender
    replayed_from VARCHAR(255)              -- Original event ID if this is a replay
);

-- Indexes for efficient querying
CREATE INDEX idx_created_at ON events (created_at);
CREATE INDEX idx_type ON events (type);
CREATE INDEX idx_status ON events (status);
CREATE INDEX idx_repository ON events (repository);
CREATE INDEX idx_sender ON events (sender);
CREATE INDEX idx_replayed_from ON events (replayed_from);
```

### Queries

All SQL queries are defined in `internal/storage/sql/queries.sql` and are validated at compile-time by SQLC. 

### Query Options
The storage interface supports filtering events by:
- Event type(s)
- Repository name
- Time range (since/until)
- Delivery status
- Sender

Example queries:
```go
// List all events
events, err := storage.ListEvents(QueryOptions{
    Types:      []string{"push", "pull_request"},
    Repository: "owner/repo",
    Since:      time.Now().Add(-24 * time.Hour),
    Status:     "delivered",
})

// List only replayed events
events, err := storage.ListEvents(QueryOptions{
    Status: "replayed",
})

// List original events that have been replayed
events, err := storage.ListEvents(QueryOptions{
    HasReplayedEvents: true,
})
```

### Querying Replay Events

You can query replayed events using the `status` filter. For example, to list all replayed events:

```go
events, err := storage.ListEvents(QueryOptions{
    Status: "replayed",
})
```

You can also query original events that have been replayed using the `HasReplayedEvents` filter:

```go
events, err := storage.ListEvents(QueryOptions{
    HasReplayedEvents: true,
})
```

### Event Replay

HubProxy allows you to replay webhook events for testing, recovery, or debugging purposes.

#### Replay ID Format

Each replayed event has an ID in the format: `original-id-replay-uuid`

For example:
- Original event ID: `d2a1f85a-delivery-id-123`
- Replayed event ID: `d2a1f85a-delivery-id-123-replay-abc123`

This format ensures:
1. Easy tracing back to original event
2. Unique IDs for multiple replays of same event
3. Clear identification of replayed events

#### Replay Single Event

```go
// Replay a single event by its ID
event, err := storage.ReplayEvent("d2a1f85a-delivery-id-123")
```

### Development Tools

1. **Development Environment** (`tools/dev.sh`)
   Sets up a complete development environment with SQLite database and test server.
   ```bash
   # Start the development environment (required before using other tools)
   ./tools/dev.sh

   # Customize webhook secret
   HUBPROXY_WEBHOOK_SECRET=my-secret ./tools/dev.sh

   # Customize test server port
   ./tools/dev.sh --target-port 8083
   ```
   This will:
   - Create a SQLite database in `.cache/hubproxy.db`
   - Start a test server to receive forwarded webhooks
   - Start the webhook proxy with GitHub IP validation disabled

   Default settings:
   - Webhook secret: `dev-secret` (via HUBPROXY_WEBHOOK_SECRET env var)
   - Test server port: 8082
   - SQLite database: `.cache/hubproxy.db`

2. **Webhook Simulator** (`internal/cmd/dev/simulate/main.go`)
   Simulates GitHub webhook events to test the proxy's handling and forwarding.
   ```bash
   # Send test webhooks with the default secret
   go run internal/cmd/dev/simulate/main.go --secret dev-secret

   # Send specific event types
   go run internal/cmd/dev/simulate/main.go --secret dev-secret --events push,pull_request

   # Add delay between events
   go run internal/cmd/dev/simulate/main.go --secret dev-secret --delay 2s
   ```

3. **Query Tool** (`internal/cmd/dev/query/main.go`)
   Inspects and analyzes webhook events stored in the database.
   ```bash
   # Show recent events
   go run internal/cmd/dev/query/main.go

   # Show event statistics
   go run internal/cmd/dev/query/main.go --stats

   # Filter by event type
   go run internal/cmd/dev/query/main.go --type push

   # Filter by repository
   go run internal/cmd/dev/query/main.go --repo "owner/repo"
   ```

4. **Test Server** (`internal/cmd/dev/testserver/main.go`)
   Simple HTTP server that logs received webhooks for verification.
   Note: You don't need to run this directly as `dev.sh` starts it for you.

   ```bash
   # Start on default port 8082
   go run internal/cmd/dev/testserver/main.go

   # Start on custom port
   go run internal/cmd/dev/testserver/main.go --port 8083
   ```

   To verify events are flowing:
   ```bash
   # Watch events in real-time
   tail -f .cache/testserver.log
   ```

### Running Tests

```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/storage/...
go test ./internal/webhook/...

# Run with race detection
go test -race ./...
```

### Testing Database Connections

```bash
# Test PostgreSQL connection
psql "postgres://user:pass@localhost:5432/hubproxy"

# Test MySQL connection
mysql -h localhost -P 3306 -u user -p hubproxy

# Test SQLite database
sqlite3 .cache/hubproxy.db
```

## API Reference

HubProxy provides a REST API for querying and replaying webhook events. All API endpoints return JSON responses.

### List Events

```http
GET /api/events
```

Lists webhook events with filtering and pagination.

**Query Parameters:**
- `type` (optional): Filter by event type (e.g., "push", "pull_request")
- `repository` (optional): Filter by repository full name (e.g., "owner/repo")
- `sender` (optional): Filter by GitHub username
- `status` (optional): Filter by event status
- `since` (optional): Start time in RFC3339 format (e.g., "2024-02-01T00:00:00Z")
- `until` (optional): End time in RFC3339 format
- `limit` (optional): Maximum number of events to return (default: 50)
- `offset` (optional): Number of events to skip for pagination

**Response:**
```json
{
  "events": [
    {
      "id": "d2a1f85a-delivery-id-123",
      "type": "push",
      "payload": {
        "ref": "refs/heads/main",
        "before": "6113728f27ae82c7b1a177c8d03f9e96e0adf246",
        "after": "76ae82c7b1a177c8d03f9e96e0adf2466113728f",
        "repository": {
          "full_name": "owner/repo",
          "private": false
        },
        "pusher": {
          "name": "username",
          "email": "user@example.com"
        }
      },
      "created_at": "2024-02-06T00:00:00Z",
      "status": "received",
      "repository": "owner/repo",
      "sender": "username"
    }
  ],
  "total": 100
}
```

### Get Event Statistics

```http
GET /api/stats
```

Returns event type statistics for a given time period.

**Query Parameters:**
- `since` (optional): Start time in RFC3339 format (default: 24 hours ago)

**Response:**
```json
{
  "push": 50,
  "pull_request": 25,
  "issues": 10
}
```

### Replay Single Event

```http
POST /api/events/{id}/replay
```

Replays a specific webhook event by its ID. The ID should be GitHub's original delivery ID.

**Response Fields:**
- `id`: Unique event ID in format `original-id-replay-uuid`
- `type`: GitHub event type (e.g., "push", "pull_request")
- `payload`: Original webhook payload from GitHub
- `created_at`: When the event was replayed
- `status`: Always "replayed" for replayed events
- `repository`: Repository full name
- `sender`: GitHub username that triggered the event
- `replayed_from`: ID of the original event that was replayed

**Response Example:**
```json
{
  "id": "d2a1f85a-delivery-id-123-replay-abc123",
  "type": "push",
  "payload": {
    "ref": "refs/heads/main",
    "before": "6113728f27ae82c7b1a177c8d03f9e96e0adf246",
    "after": "76ae82c7b1a177c8d03f9e96e0adf2466113728f",
    "repository": {
      "full_name": "owner/repo",
      "private": false
    },
    "pusher": {
      "name": "username",
      "email": "user@example.com"
    }
  },
  "created_at": "2024-02-06T00:00:00Z",
  "status": "replayed",
  "repository": "owner/repo",
  "sender": "username",
  "replayed_from": "d2a1f85a-delivery-id-123"
}
```

### Replay Events by Time Range

```http
POST /api/replay
```

Replays all webhook events within a specified time range.

**Query Parameters:**
- `since` (required): Start time in RFC3339 format (e.g., "2024-02-01T00:00:00Z")
- `until` (required): End time in RFC3339 format
- `type` (optional): Filter by event type
- `repository` (optional): Filter by repository full name
- `sender` (optional): Filter by GitHub username

**Response Fields:**
- `replayed_count`: Number of events replayed
- `events`: List of replayed events with:
  - `id`: Unique event ID in format `original-id-replay-uuid`
  - `type`: GitHub event type (e.g., "push", "pull_request")
  - `payload`: Original webhook payload from GitHub
  - `created_at`: When the event was replayed
  - `status`: Always "replayed" for replayed events
  - `repository`: Repository full name
  - `sender`: GitHub username that triggered the event
  - `replayed_from`: ID of the original event that was replayed

**Response Example:**
```json
{
  "replayed_count": 5,
  "events": [
    {
      "id": "d2a1f85a-delivery-id-123-replay-abc123",
      "type": "push",
      "payload": {
        "ref": "refs/heads/main",
        "before": "6113728f27ae82c7b1a177c8d03f9e96e0adf246",
        "after": "76ae82c7b1a177c8d03f9e96e0adf2466113728f",
        "repository": {
          "full_name": "owner/repo",
          "private": false
        },
        "pusher": {
          "name": "username",
          "email": "user@example.com"
        }
      },
      "created_at": "2024-02-06T00:00:00Z",
      "status": "replayed",
      "repository": "owner/repo",
      "sender": "username",
      "replayed_from": "d2a1f85a-delivery-id-123"
    },
    ...
  ]
}
```

**Notes:**
- Each replayed event uses GitHub's original delivery ID to ensure proper tracing
- The event is marked with a "replayed" status
- The original event remains unchanged in the database
- The webhook payload is preserved exactly as it was in the original event
- Range replay has a default limit of 100 events (can be overridden with `limit` parameter)

## Configuration

HubProxy can be configured using either command-line flags or a YAML configuration file, with sensitive values like secrets being managed through environment variables. When both configuration methods are used, command-line flags take precedence over the configuration file.

### Environment Variables

Sensitive configuration values should be provided through environment variables:

- `HUBPROXY_WEBHOOK_SECRET`: GitHub webhook secret for verification (required)

### Configuration File

Create a `config.yaml` file (see `config.example.yaml` for a template) with your desired settings.

```yaml
# Target URL to forward webhooks to
target-url: "http://your-service:8080/webhook"

# Log level (debug, info, warn, error)
log-level: info

# Validate that requests come from GitHub IPs
validate-ip: true

# Tailscale configuration (optional)
ts-authkey: ""
ts-hostname: hubproxy

# Database configuration
db: sqlite:hubproxy.db
# db: mysql://user:pass@host/db
# db: postgres://user:pass@host/db
```

To use a configuration file, specify its path with the `--config` flag:

```bash
export HUBPROXY_WEBHOOK_SECRET="your-secret-here"
hubproxy --config config.yaml
```

### Command Line Flags

Most configuration options can also be set via command-line flags:

- `--config`: Path to config file (optional)
- `--target`: Target URL to forward webhooks to
- `--log-level`: Log level (debug, info, warn, error)
- `--validate-ip`: Validate that requests come from GitHub IPs
- `--ts-authkey`: Tailscale auth key for tsnet
- `--ts-hostname`: Tailscale hostname
- `--db`: Database URI (e.g., sqlite:file.db, mysql://user:pass@host/db, postgresql://user:pass@host/db)

Command-line flags take precedence over values in the configuration file.

## Security Features

### Webhook Signature Verification

Every webhook request is verified using GitHub's HMAC-SHA256 signature to ensure it hasn't been tampered with. The signature is provided in the `X-Hub-Signature-256` header and verified against your webhook secret.

### GitHub IP Range Validation

HubProxy can optionally validate that webhook requests come from GitHub's dynamic IP ranges. 
Many app implementations for GitHub miss this useful verification step, so we do it
automatically. 

- Automatically fetches and caches GitHub's webhook IP ranges from the `/meta` API
- Updates the IP ranges hourly (configurable)
- Rejects requests from non-GitHub IP addresses
- Provides additional security beyond webhook signatures

Enable/disable IP validation using the `-validate-ip` flag:
```bash
# Enable IP validation (default)
hubproxy -validate-ip

# Disable IP validation (useful for local development)
hubproxy -validate-ip=false
```

Note: When running behind a proxy or load balancer, ensure it's configured to forward the original client IP (e.g., using X-Forwarded-For header).

### Tailscale Configuration

HubProxy optionally uses Tailscale's Funnel feature to expose the service publicly, allowing GitHub to send webhooks to it. The service listens on port 443 (HTTPS) and Tailscale handles all SSL/TLS termination.

To use this feature:

1. Generate an auth key from your [Tailscale Admin Console](https://login.tailscale.com/admin/settings/keys)
2. Run hubproxy with the following flags:
   ```bash
   hubproxy --ts-authkey=ts-abc123... --ts-hostname=hubproxy
   ```

Your proxy will be accessible at `hubproxy.<tailnet>.ts.net`. You can customize the hostname using the `--ts-hostname` flag.

This is useful for:
- Running hubproxy in a private network without exposing it to the internet
- Accessing hubproxy from any device in your Tailscale network
- Using Tailscale's ACLs to control access to the proxy

## Architecture

```
GitHub Webhook ──→ HubProxy ──→ Your Application
                        │
                        ↓
                     Database

1. GitHub sends a webhook to HubProxy
2. HubProxy verifies the webhook signature
3. The event is stored in a database
4. If configured, the webhook is forwarded to your application
5. The delivery status is updated in the database
```

### Why a database as a Canonical Store?

Using a database as the source of truth for webhook events provides several key benefits:

1. **Complete Event History**
   - GitHub only keeps webhooks for 30 days, so if you want to retry after that you're out of luck
   - Full control over data retention
   - Comprehensive audit trail
   - Historical analysis capabilities

2. **Reliable Delivery**
   - Events persisted even if your app is down
   - Replay capabilities for recovery
   - No missing webhooks during outages
   - Exactly-once delivery possible

3. **Rich Querying**
   ```sql
   -- Find PRs affecting specific files (PostgreSQL example)
   SELECT repository, payload->>'action', created_at 
   FROM events 
   WHERE type = 'pull_request' 
   AND json_extract(payload, '$.pull_request.changed_files.filename') = 'critical.js';

   -- Track deployment frequency (works in all databases)
   SELECT date(created_at), count(*) 
   FROM events 
   WHERE type = 'deployment' 
   GROUP BY 1;
   ```

4. **Operational Excellence**
   - Standard backup/restore procedures
   - Replication for high availability
   - Familiar tooling and ecosystem
   - Easy integration with existing systems


## Contributing

Contributions are welcome! Here's how you can help:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run the tests (`go test ./...`)
5. Run the linter (`golangci-lint run ./...`)
6. Commit your changes (`git commit -am 'Add amazing feature'`)
7. Push to the branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

Please make sure your PR:
- Includes tests for new functionality
- Updates documentation as needed
- Follows the existing code style
- Includes a clear description of the changes

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
