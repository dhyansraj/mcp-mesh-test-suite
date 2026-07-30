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

	workdir := stepWorkdir(step, ctx)

	timeout := parseDuration(step["timeout"], defaultShellTimeout)

	return runShellCommand(interpolatedCmd, workdir, timeout)
}

// defaultShellTimeout bounds a single shell command when the step does not set one.
const defaultShellTimeout = 120 * time.Second

// stepWorkdir resolves the directory a command runs in: the step's own workdir,
// else the context workdir, else /workspace. The result is interpolated.
func stepWorkdir(step map[string]any, ctx *interpolate.Context) string {
	workdir := "/workspace"
	if w, ok := step["workdir"].(string); ok && w != "" {
		workdir = w
	} else if ctx.Workdir != "" {
		workdir = ctx.Workdir
	}
	workdir, _ = interpolate.Interpolate(workdir, ctx)
	return workdir
}

// runShellCommand executes an already-interpolated command with bash and
// captures its output. It is shared by the shell and probe handlers so both get
// identical environment, working directory, and exit-code semantics.
//
// A non-zero exit is reported as Success=false with an empty Error: the caller
// decides whether that is fatal (shell) or just a failed attempt (probe). Only
// a timeout or a failure to start the process sets Error here.
func runShellCommand(command, workdir string, timeout time.Duration) StepResult {
	cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", command)
	cmd.Dir = workdir

	cmd.Env = os.Environ()
	if apiURL := os.Getenv("TSUITE_API"); apiURL != "" {
		cmd.Env = append(cmd.Env, "TSUITE_API="+apiURL)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return StepResult{
				Success:  false,
				ExitCode: 124,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Error:    fmt.Sprintf("command timed out after %v", timeout),
			}
		} else if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
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
