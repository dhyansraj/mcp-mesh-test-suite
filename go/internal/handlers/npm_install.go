package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
