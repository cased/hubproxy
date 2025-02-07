#!/bin/bash
set -e

# Default configuration
DEFAULT_SECRET="dev-secret"
DEFAULT_TARGET_PORT=8082
DB_PATH=".cache/hubproxy.db"

# Parse arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --target-port) TARGET_PORT="$2"; shift ;;
        --help) 
            echo "Usage: $0 [--target-port PORT]"
            echo "Starts HubProxy development environment with:"
            echo "  1. SQLite database"
            echo "  2. Test server (for receiving forwarded webhooks)"
            echo "  3. Webhook proxy"
            echo ""
            echo "Options:"
            echo "  --target-port  Port for test server (default: 8082)"
            exit 0
            ;;
        *) echo "Unknown parameter: $1"; exit 1 ;;
    esac
    shift
done

TARGET_PORT=${TARGET_PORT:-$DEFAULT_TARGET_PORT}

# Ensure we're in the project root
cd "$(dirname "$0")/.."

echo "🚀 Starting HubProxy development environment..."

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    # Kill test server
    if [ -n "$TEST_SERVER_PID" ]; then
        kill $TEST_SERVER_PID 2>/dev/null || true
    fi
    # Kill any existing proxy processes
    pkill -f "hubproxy.*--target" || true
}

# Set up cleanup on script exit
trap cleanup EXIT

# Run cleanup on start to ensure no leftover processes
cleanup

# Create cache directory if it doesn't exist
mkdir -p "$(dirname "$DB_PATH")"

# Export development environment variables
echo "Previous webhook secret: $HUBPROXY_WEBHOOK_SECRET"
export HUBPROXY_WEBHOOK_SECRET=${HUBPROXY_WEBHOOK_SECRET:-$DEFAULT_SECRET}
echo "Using webhook secret: $HUBPROXY_WEBHOOK_SECRET (default: $DEFAULT_SECRET)"

# Start the test server in the background
echo "Starting test server..."
go run internal/cmd/dev/testserver/main.go --port $TARGET_PORT > .cache/testserver.log 2>&1 &
TEST_SERVER_PID=$!

# Wait for test server to be ready
echo "Waiting for test server..."
sleep 2

# Start the proxy
echo "Starting webhook proxy..."
go run cmd/proxy/main.go \
    --target "http://localhost:$TARGET_PORT" \
    --db sqlite \
    --db-dsn "$DB_PATH" \
    --validate-ip=false \
    --log-level debug
