#!/bin/bash
# Run tsuite CLI

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

# Default suite path if not provided
if [[ ! "$*" =~ "--suite-path" ]]; then
    echo "Running CLI with default suite path: ../integration/suites"
    python -m tsuite.cli --suite-path "$PROJECT_ROOT/integration/suites" "$@"
else
    python -m tsuite.cli "$@"
fi
