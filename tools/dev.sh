#!/bin/bash
set -e

# Default configuration
DEFAULT_SECRET="dev-secret"
DEFAULT_TARGET_PORT=8082
DB_PATH=".cache/hubproxy.db"

# Parse arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --secret) SECRET="$2"; shift ;;
        --target-port) TARGET_PORT="$2"; shift ;;
        --help) 
            echo "Usage: $0 [--secret SECRET] [--target-port PORT]"
            echo "Starts HubProxy development environment with:"
            echo "  1. SQLite database"
            echo "  2. Test server (for receiving forwarded webhooks)"
            echo "  3. Webhook proxy"
            echo ""
            echo "Options:"
            echo "  --secret       Webhook secret (default: dev-secret)"
            echo "  --target-port  Port for test server (default: 8082)"
            exit 0
            ;;
        *) echo "Unknown parameter: $1"; exit 1 ;;
    esac
    shift
done

SECRET=${SECRET:-$DEFAULT_SECRET}
TARGET_PORT=${TARGET_PORT:-$DEFAULT_TARGET_PORT}

# Ensure we're in the project root
cd "$(dirname "$0")/.."

echo "🚀 Starting HubProxy development environment..."

# Create cache directory if it doesn't exist
mkdir -p "$(dirname "$DB_PATH")"

# Export development environment variables
export GITHUB_WEBHOOK_SECRET=$SECRET

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
    --secret $SECRET \
    --target "http://localhost:$TARGET_PORT" \
    --db sqlite \
    --db-dsn "$DB_PATH" \
    --validate-ip=false

# Cleanup function
cleanup() {
    echo -e "\n🧹 Cleaning up..."
    kill $TEST_SERVER_PID 2>/dev/null || true
    rm -f .cache/testserver.log
    echo "✨ Done"
}

# Set up cleanup on script exit
trap cleanup EXIT
