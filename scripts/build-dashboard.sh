#!/bin/bash
# Build the dashboard and copy static files to tsuite package

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

DASHBOARD_DIR="$PROJECT_ROOT/dashboard"
TSUITE_DASHBOARD_DIR="$PROJECT_ROOT/tsuite/tsuite/dashboard"

echo "Building dashboard..."
cd "$DASHBOARD_DIR"

# Install dependencies if needed
if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install
fi

# Build static export
echo "Creating static export..."
npm run build

# Copy to tsuite package
echo "Copying to tsuite package..."
rm -rf "$TSUITE_DASHBOARD_DIR"
cp -r "$DASHBOARD_DIR/out" "$TSUITE_DASHBOARD_DIR"

# Create a marker file with build info
echo "Build date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$TSUITE_DASHBOARD_DIR/.build-info"

echo ""
echo "Dashboard built and copied to: $TSUITE_DASHBOARD_DIR"
echo "Files: $(find "$TSUITE_DASHBOARD_DIR" -type f | wc -l | tr -d ' ')"
echo "Size: $(du -sh "$TSUITE_DASHBOARD_DIR" | cut -f1)"
