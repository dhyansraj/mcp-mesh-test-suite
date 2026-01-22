#!/bin/bash
# Start tsuite components
#
# Usage:
#   ./start.sh --api          Start API server (port 9999)
#   ./start.sh --ui           Start dashboard UI (port 3000)
#   ./start.sh --cli [args]   Run CLI with optional arguments
#   ./start.sh --all          Start API and UI (in background), then CLI
#
# Examples:
#   ./start.sh --api
#   ./start.sh --ui
#   ./start.sh --cli --uc uc01_registry
#   ./start.sh --cli --suite-path /path/to/suite --docker

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

show_help() {
    echo "Usage: ./start.sh [OPTION] [ARGS...]"
    echo ""
    echo "Options:"
    echo "  --api          Start API server (port 9999)"
    echo "  --ui           Start dashboard UI (port 3000)"
    echo "  --cli [args]   Run CLI with optional arguments"
    echo "  --all          Start API and UI in background"
    echo "  --clear        Clear all data (database + run logs)"
    echo "  --help         Show this help message"
    echo ""
    echo "CLI Examples:"
    echo "  ./start.sh --cli                              # Run all tests"
    echo "  ./start.sh --cli --uc uc01_registry           # Run specific use case"
    echo "  ./start.sh --cli --tc tc01_simple             # Run specific test case"
    echo "  ./start.sh --cli --standalone                 # Run without Docker"
    echo ""
    echo "Maintenance:"
    echo "  ./start.sh --clear                            # Clear DB and logs"
    echo ""
}

if [ $# -eq 0 ]; then
    show_help
    exit 0
fi

case "$1" in
    --api)
        shift
        exec "$SCRIPT_DIR/scripts/start-api.sh" "$@"
        ;;
    --ui)
        shift
        exec "$SCRIPT_DIR/scripts/start-ui.sh" "$@"
        ;;
    --cli)
        shift
        exec "$SCRIPT_DIR/scripts/start-cli.sh" "$@"
        ;;
    --all)
        echo "Starting API server in background..."
        "$SCRIPT_DIR/scripts/start-api.sh" &
        API_PID=$!
        sleep 2

        echo "Starting UI in background..."
        "$SCRIPT_DIR/scripts/start-ui.sh" &
        UI_PID=$!

        echo ""
        echo "Services started:"
        echo "  API: http://localhost:9999 (PID: $API_PID)"
        echo "  UI:  http://localhost:3000 (PID: $UI_PID)"
        echo ""
        echo "Press Ctrl+C to stop all services"

        trap "kill $API_PID $UI_PID 2>/dev/null" EXIT
        wait
        ;;
    --help|-h)
        show_help
        ;;
    --clear)
        echo "Clearing tsuite data..."

        # Remove database
        if [ -f "$HOME/.tsuite/results.db" ]; then
            rm -f "$HOME/.tsuite/results.db"
            echo "  Removed: ~/.tsuite/results.db"
        fi

        # Remove run logs
        if [ -d "$HOME/.tsuite/runs" ]; then
            rm -rf "$HOME/.tsuite/runs"
            echo "  Removed: ~/.tsuite/runs/"
        fi

        # Remove meshctl logs (standalone mode logs)
        if [ -d "$HOME/.mcp-mesh/logs" ]; then
            rm -rf "$HOME/.mcp-mesh/logs"/*
            echo "  Cleared: ~/.mcp-mesh/logs/"
        fi

        echo "Done."
        ;;
    *)
        echo "Unknown option: $1"
        show_help
        exit 1
        ;;
esac
