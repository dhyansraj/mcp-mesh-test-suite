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

// PipInstallHandler installs Python packages
type PipInstallHandler struct{}

func (h *PipInstallHandler) Name() string {
	return "pip-install"
}

func (h *PipInstallHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	// Get path (for requirements.txt) or packages list
	path, hasPath := step["path"].(string)
	packages, hasPackages := step["packages"].([]any)

	if !hasPath && !hasPackages {
		return StepResult{
			Success: false,
			Error:   "pip-install handler requires 'path' (for requirements.txt) or 'packages' field",
		}
	}

	timeout := 300 * time.Second
	if t, ok := step["timeout"].(int); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	// Determine package mode (local or published)
	mode := "published"
	if _, err := os.Stat("/wheels"); err == nil {
		mode = "local"
	}

	if hasPath {
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

		// Check if requirements.txt exists
		requirementsFile := path
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			requirementsFile = filepath.Join(path, "requirements.txt")
		}

		if _, err := os.Stat(requirementsFile); os.IsNotExist(err) {
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Error:    fmt.Sprintf("requirements.txt not found at %s", requirementsFile),
			}
		}

		if mode == "local" {
			// Local mode: install from local wheels first
			cmd := exec.CommandContext(cmdCtx, "bash", "-c", fmt.Sprintf(`
				# Install from local wheels if available
				if [ -d /wheels ]; then
					echo "Using local wheels from /wheels"
					pip install --find-links=/wheels --no-index /wheels/*.whl 2>/dev/null || true
				fi

				# Install remaining packages from requirements.txt
				pip install -r "%s"
			`, requirementsFile))

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
						Error:    "pip install timed out",
					}
				}
				return StepResult{
					Success:  false,
					ExitCode: 1,
					Stdout:   stdout.String(),
					Stderr:   stderr.String(),
					Error:    fmt.Sprintf("pip install failed: %v", err),
				}
			}
		} else {
			// Published mode: just run pip install
			cmd := exec.CommandContext(cmdCtx, "pip", "install", "-r", requirementsFile)
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
						Error:    "pip install timed out",
					}
				}
				return StepResult{
					Success:  false,
					ExitCode: 1,
					Stdout:   stdout.String(),
					Stderr:   stderr.String(),
					Error:    fmt.Sprintf("pip install failed: %v", err),
				}
			}
		}
	} else if hasPackages {
		// Install specific packages
		pkgList := make([]string, 0, len(packages))
		for _, p := range packages {
			if ps, ok := p.(string); ok {
				ps, _ = interpolate.Interpolate(ps, ctx)
				pkgList = append(pkgList, ps)
			}
		}

		args := append([]string{"install"}, pkgList...)
		if mode == "local" {
			args = append([]string{"install", "--find-links=/wheels"}, pkgList...)
		}

		cmd := exec.CommandContext(cmdCtx, "pip", args...)
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
					Error:    "pip install timed out",
				}
			}
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Error:    fmt.Sprintf("pip install failed: %v", err),
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
