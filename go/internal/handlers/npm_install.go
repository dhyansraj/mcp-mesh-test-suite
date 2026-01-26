package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// NpmInstallHandler installs npm packages
type NpmInstallHandler struct{}

func (h *NpmInstallHandler) Name() string {
	return "npm-install"
}

func (h *NpmInstallHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	// Get path
	path, _ := step["path"].(string)
	if path == "" {
		return StepResult{
			Success: false,
			Error:   "npm-install handler requires 'path' field",
		}
	}

	// Interpolate path
	path, _ = interpolate.Interpolate(path, ctx)

	// Make path absolute if not already
	if !filepath.IsAbs(path) {
		workdir := ctx.Workdir
		if workdir == "" {
			workdir = "/workspace"
		}
		path = filepath.Join(workdir, path)
	}

	// Check if package.json exists
	packageJSON := filepath.Join(path, "package.json")
	if _, err := os.Stat(packageJSON); os.IsNotExist(err) {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("package.json not found at %s", path),
		}
	}

	// Strip file: dependencies by default (set strip_file_deps: false to disable)
	stripFileDeps := true
	if v, ok := step["strip_file_deps"].(bool); ok {
		stripFileDeps = v
	}

	if stripFileDeps {
		if err := stripFileDepependencies(packageJSON); err != nil {
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Error:    fmt.Sprintf("failed to strip file: dependencies: %v", err),
			}
		}
	}

	// Determine package mode (local or published)
	// Check for /packages directory (local mode) or default to published
	mode := "published"
	if _, err := os.Stat("/packages"); err == nil {
		mode = "local"
	}

	timeout := 300 * time.Second
	if t, ok := step["timeout"].(int); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	if mode == "local" {
		// Local mode: npm install with local packages
		// First, install local packages, then run npm install
		cmd := exec.CommandContext(cmdCtx, "bash", "-c", `
			cd "$1"

			# Check if local packages exist
			if [ -d /packages ]; then
				echo "Using local packages from /packages"

				# Install @mcpmesh packages from local tarballs
				for pkg in /packages/*.tgz; do
					if [ -f "$pkg" ]; then
						echo "Installing local package: $pkg"
						npm install "$pkg" --save --legacy-peer-deps 2>/dev/null || true
					fi
				done
			fi

			# Run standard npm install
			npm install --legacy-peer-deps
		`, "bash", path)

		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err != nil {
			if cmdCtx.Err() == context.DeadlineExceeded {
				return StepResult{
					Success:  false,
					ExitCode: 124,
					Stdout:   stdout.String(),
					Stderr:   stderr.String(),
					Error:    "npm install timed out",
				}
			}
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Error:    fmt.Sprintf("npm install failed: %v", err),
			}
		}
	} else {
		// Published mode: just run npm install
		cmd := exec.CommandContext(cmdCtx, "npm", "install", "--legacy-peer-deps")
		cmd.Dir = path
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err != nil {
			if cmdCtx.Err() == context.DeadlineExceeded {
				return StepResult{
					Success:  false,
					ExitCode: 124,
					Stdout:   stdout.String(),
					Stderr:   stderr.String(),
					Error:    "npm install timed out",
				}
			}
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Error:    fmt.Sprintf("npm install failed: %v", err),
			}
		}
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

// stripFileDepependencies removes file: dependencies from package.json
// This is useful when examples reference local packages via file: paths
// that don't exist in the container. Local .tgz packages from /packages
// will provide these dependencies instead.
func stripFileDepependencies(packageJSONPath string) error {
	// Read package.json
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return fmt.Errorf("failed to read package.json: %w", err)
	}

	// Parse as generic map to preserve structure
	var pkg map[string]any
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("failed to parse package.json: %w", err)
	}

	modified := false

	// Strip file: deps from dependencies
	if deps, ok := pkg["dependencies"].(map[string]any); ok {
		for name, version := range deps {
			if v, ok := version.(string); ok && strings.HasPrefix(v, "file:") {
				delete(deps, name)
				modified = true
			}
		}
	}

	// Strip file: deps from devDependencies
	if deps, ok := pkg["devDependencies"].(map[string]any); ok {
		for name, version := range deps {
			if v, ok := version.(string); ok && strings.HasPrefix(v, "file:") {
				delete(deps, name)
				modified = true
			}
		}
	}

	// Strip file: deps from optionalDependencies
	if deps, ok := pkg["optionalDependencies"].(map[string]any); ok {
		for name, version := range deps {
			if v, ok := version.(string); ok && strings.HasPrefix(v, "file:") {
				delete(deps, name)
				modified = true
			}
		}
	}

	// Strip file: deps from peerDependencies
	if deps, ok := pkg["peerDependencies"].(map[string]any); ok {
		for name, version := range deps {
			if v, ok := version.(string); ok && strings.HasPrefix(v, "file:") {
				delete(deps, name)
				modified = true
			}
		}
	}

	// Only write if modified
	if modified {
		// Marshal with indentation to preserve readability
		newData, err := json.MarshalIndent(pkg, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal package.json: %w", err)
		}

		// Write back
		if err := os.WriteFile(packageJSONPath, newData, 0644); err != nil {
			return fmt.Errorf("failed to write package.json: %w", err)
		}
	}

	return nil
}
