package api

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/gin-gonic/gin"
)

// Version is the build-time version, set by main.go.
// Used to fetch the correct npm package version for on-demand runner downloads.
var Version string

// selectRunnerScript is the embedded select-runner.sh script (674 bytes).
const selectRunnerScript = `#!/bin/sh
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
`

// npmPackageMap maps runner binary names to npm package names.
var npmPackageMap = map[string]string{
	"tsuite-runner-linux-amd64":  "@mcpmesh/tsuite-linux-x64",
	"tsuite-runner-linux-arm64":  "@mcpmesh/tsuite-linux-arm64",
	"tsuite-runner-darwin-amd64": "@mcpmesh/tsuite-darwin-x64",
	"tsuite-runner-darwin-arm64": "@mcpmesh/tsuite-darwin-arm64",
}

// knownRunners is the list of all runner names that can be served on-demand.
var knownRunners = []string{
	"tsuite-runner-linux-amd64",
	"tsuite-runner-linux-arm64",
	"tsuite-runner-darwin-amd64",
	"tsuite-runner-darwin-arm64",
	"select-runner",
}

var (
	fetchMu      sync.Mutex
	fetchLocks   = make(map[string]*sync.Mutex)
)

// lockForRunner returns a per-runner mutex to prevent concurrent duplicate downloads.
func lockForRunner(name string) *sync.Mutex {
	fetchMu.Lock()
	defer fetchMu.Unlock()
	mu, ok := fetchLocks[name]
	if !ok {
		mu = &sync.Mutex{}
		fetchLocks[name] = mu
	}
	return mu
}

func cacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tsuite", "cache", "runners", Version)
}

// fetchRunner downloads a runner binary on-demand via npm, caching it locally.
func fetchRunner(name string) ([]byte, error) {
	if Version == "" || Version == "dev" {
		return nil, fmt.Errorf("on-demand runner fetch requires a versioned build (current: %q). Build with: make build-with-version VERSION=x.y.z", Version)
	}

	// select-runner is served directly from the embedded constant
	if name == "select-runner" {
		return []byte(selectRunnerScript), nil
	}

	npmPkg, ok := npmPackageMap[name]
	if !ok {
		return nil, fmt.Errorf("unknown runner: %s", name)
	}

	// Per-runner lock to prevent concurrent duplicate downloads
	mu := lockForRunner(name)
	mu.Lock()
	defer mu.Unlock()

	// Check local cache
	cachePath := filepath.Join(cacheDir(), name)
	if data, err := os.ReadFile(cachePath); err == nil {
		return data, nil
	}

	// Download via npm pack
	tmpDir, err := os.MkdirTemp("", "tsuite-runner-download-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	pkgRef := fmt.Sprintf("%s@%s", npmPkg, Version)
	packCmd := exec.Command("npm", "pack", pkgRef, "--pack-destination", tmpDir)
	packOut, err := packCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("npm pack %s failed: %w\n%s", pkgRef, err, string(packOut))
	}

	// Find the tarball (npm pack outputs the filename)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp dir: %w", err)
	}
	var tarball string
	for _, e := range entries {
		if !e.IsDir() {
			tarball = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if tarball == "" {
		return nil, fmt.Errorf("npm pack produced no output file")
	}

	// Extract the runner binary from the tarball
	tarCmd := exec.Command("tar", "xf", tarball, "--to-stdout", "package/bin/tsuite-runner")
	data, err := tarCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to extract runner from tarball: %w", err)
	}

	// Cache to disk
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}
	if err := os.WriteFile(cachePath, data, 0755); err != nil {
		return nil, fmt.Errorf("failed to write cache file: %w", err)
	}

	return data, nil
}

// listFetchableRunners returns the list of all known runners with their sizes.
// Size is 0 if not cached, actual file size if cached.
func listFetchableRunners() []gin.H {
	var runners []gin.H
	cd := cacheDir()

	for _, name := range knownRunners {
		var size int64
		if name == "select-runner" {
			size = int64(len(selectRunnerScript))
		} else {
			cachePath := filepath.Join(cd, name)
			if info, err := os.Stat(cachePath); err == nil {
				size = info.Size()
			}
		}
		runners = append(runners, gin.H{
			"name": name,
			"size": size,
		})
	}

	return runners
}
