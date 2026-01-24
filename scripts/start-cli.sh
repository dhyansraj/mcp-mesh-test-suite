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

# Process arguments - resolve relative paths for --suite-path
ARGS=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --suite-path)
            shift
            if [[ -n "$1" && ! "$1" =~ ^- ]]; then
                # Resolve relative paths from project root
                if [[ "$1" = /* ]]; then
                    ARGS+=("--suite-path" "$1")
                else
                    ARGS+=("--suite-path" "$PROJECT_ROOT/$1")
                fi
                shift
            else
                ARGS+=("--suite-path")
            fi
            ;;
        *)
            ARGS+=("$1")
            shift
            ;;
    esac
done

# Default suite path if not provided
if [[ ! " ${ARGS[*]} " =~ " --suite-path " ]]; then
    ARGS+=("--suite-path" "$PROJECT_ROOT/integration/suites")
fi

python -m tsuite.cli "${ARGS[@]}"
