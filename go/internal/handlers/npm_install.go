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

	// Replace file: dependencies by default (set replace_file_deps: false to disable)
	replaceFileDeps := true
	if v, ok := step["replace_file_deps"].(bool); ok {
		replaceFileDeps = v
	}
	// Legacy support for strip_file_deps
	if v, ok := step["strip_file_deps"].(bool); ok {
		replaceFileDeps = v
	}

	if replaceFileDeps {
		// Get version from config if available, otherwise use "*"
		version := "*"
		if packages, ok := ctx.Config["packages"].(map[string]any); ok {
			if v, ok := packages["sdk_typescript_version"].(string); ok && v != "" {
				version = v
			}
		}

		if err := replaceFileDepependencies(packageJSON, version); err != nil {
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Error:    fmt.Sprintf("failed to replace file: dependencies: %v", err),
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
		// First run npm install, then override with local packages
		cmd := exec.CommandContext(cmdCtx, "bash", "-c", `
			cd "$1"

			# Run standard npm install first
			npm install --legacy-peer-deps

			# Then override with local packages if they exist
			if [ -d /packages ]; then
				echo "Overriding with local packages from /packages"

				# Install @mcpmesh packages from local tarballs (overrides npm versions)
				for pkg in /packages/*.tgz; do
					if [ -f "$pkg" ]; then
						echo "Installing local package: $pkg"
						npm install "$pkg" --save --legacy-peer-deps
					fi
				done
			fi
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

// replaceFileDepependencies replaces file: dependencies in package.json with a version
// This is useful when examples reference local packages via file: paths
// that don't exist in the container. The version is replaced so npm install
// can resolve the package, and local .tgz packages can override afterward.
func replaceFileDepependencies(packageJSONPath string, version string) error {
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

	// Replace file: deps in dependencies
	if deps, ok := pkg["dependencies"].(map[string]any); ok {
		for name, ver := range deps {
			if v, ok := ver.(string); ok && strings.HasPrefix(v, "file:") {
				deps[name] = version
				modified = true
			}
		}
	}

	// Replace file: deps in devDependencies
	if deps, ok := pkg["devDependencies"].(map[string]any); ok {
		for name, ver := range deps {
			if v, ok := ver.(string); ok && strings.HasPrefix(v, "file:") {
				deps[name] = version
				modified = true
			}
		}
	}

	// Replace file: deps in optionalDependencies
	if deps, ok := pkg["optionalDependencies"].(map[string]any); ok {
		for name, ver := range deps {
			if v, ok := ver.(string); ok && strings.HasPrefix(v, "file:") {
				deps[name] = version
				modified = true
			}
		}
	}

	// Replace file: deps in peerDependencies
	if deps, ok := pkg["peerDependencies"].(map[string]any); ok {
		for name, ver := range deps {
			if v, ok := ver.(string); ok && strings.HasPrefix(v, "file:") {
				deps[name] = version
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
