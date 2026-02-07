#!/bin/sh
# select-runner.sh - Select the correct architecture runner binary
# Usage: select-runner.sh [runner-dir] [args...]
#   runner-dir: directory containing runner binaries (default: current dir)
#   args: arguments to pass to the runner
#
# Looks for tsuite-runner-{os}-{arch} in the runner directory.
# Falls back to tsuite-runner if arch-specific binary not found.

RUNNER_DIR="${1:-.}"
shift
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
RUNNER="${RUNNER_DIR}/tsuite-runner-${OS}-${ARCH}"
[ ! -f "$RUNNER" ] && RUNNER="${RUNNER_DIR}/tsuite-runner"
exec "$RUNNER" "$@"
