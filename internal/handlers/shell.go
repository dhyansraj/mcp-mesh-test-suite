package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// ShellHandler executes shell commands
type ShellHandler struct{}

func (h *ShellHandler) Name() string {
	return "shell"
}

func (h *ShellHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	// Get command
	command, _ := step["command"].(string)
	if command == "" {
		return StepResult{
			Success: false,
			Error:   "shell handler requires 'command' field",
		}
	}

	// Interpolate command
	interpolatedCmd, err := interpolate.Interpolate(command, ctx)
	if err != nil {
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("failed to interpolate command: %v", err),
		}
	}

	// Get workdir
	workdir := "/workspace"
	if w, ok := step["workdir"].(string); ok && w != "" {
		workdir = w
	} else if ctx.Workdir != "" {
		workdir = ctx.Workdir
	}

	// Interpolate workdir
	workdir, _ = interpolate.Interpolate(workdir, ctx)

	// Get timeout
	timeout := 120 * time.Second
	if t, ok := step["timeout"].(int); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}

	// Create command context with timeout
	cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Execute command with bash
	cmd := exec.CommandContext(cmdCtx, "bash", "-c", interpolatedCmd)
	cmd.Dir = workdir

	// Set up environment
	cmd.Env = os.Environ()
	if apiURL := os.Getenv("TSUITE_API"); apiURL != "" {
		cmd.Env = append(cmd.Env, "TSUITE_API="+apiURL)
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err = cmd.Run()

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else if cmdCtx.Err() == context.DeadlineExceeded {
			return StepResult{
				Success:  false,
				ExitCode: 124,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Error:    fmt.Sprintf("command timed out after %v", timeout),
			}
		} else {
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Error:    err.Error(),
			}
		}
	}

	return StepResult{
		Success:  exitCode == 0,
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Error:    "",
	}
}
