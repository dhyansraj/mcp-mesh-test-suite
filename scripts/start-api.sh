#!/bin/bash
# Start the tsuite API server (port 9999)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT/tsuite"

# Activate virtual environment
if [ -d "venv" ]; then
    source venv/bin/activate
else
    echo "Error: venv not found. Run setup first:"
    echo "  cd tsuite && python3 -m venv venv && source venv/bin/activate && pip install -r requirements.txt -e ."
    exit 1
fi

echo "Starting API server on http://localhost:9999..."
python -m tsuite.server "$@"
