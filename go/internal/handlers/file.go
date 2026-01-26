package handlers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// FileHandler handles file operations
type FileHandler struct{}

func (h *FileHandler) Name() string {
	return "file"
}

func (h *FileHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	operation := "exists"
	if op, ok := step["operation"].(string); ok && op != "" {
		operation = op
	}

	path, _ := step["path"].(string)
	if path == "" {
		return StepResult{
			Success: false,
			Error:   "file handler requires 'path' field",
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

	switch operation {
	case "exists":
		return h.opExists(path)
	case "read":
		return h.opRead(path)
	case "write":
		return h.opWrite(path, step, ctx)
	case "delete":
		return h.opDelete(path)
	case "mkdir":
		return h.opMkdir(path)
	default:
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("unknown file operation: %s", operation),
		}
	}
}

func (h *FileHandler) opExists(path string) StepResult {
	_, err := os.Stat(path)
	exists := err == nil

	return StepResult{
		Success:  exists,
		ExitCode: boolToInt(!exists),
		Stdout:   fmt.Sprintf("%v", exists),
		Error:    errorIf(!exists, fmt.Sprintf("path does not exist: %s", path)),
	}
}

func (h *FileHandler) opRead(path string) StepResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    err.Error(),
		}
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   string(data),
	}
}

func (h *FileHandler) opWrite(path string, step map[string]any, ctx *interpolate.Context) StepResult {
	content, _ := step["content"].(string)

	// Interpolate content
	content, _ = interpolate.Interpolate(content, ctx)

	// Create parent directories
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("failed to create directory: %v", err),
		}
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    err.Error(),
		}
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   fmt.Sprintf("Wrote to %s", path),
	}
}

func (h *FileHandler) opDelete(path string) StepResult {
	if err := os.RemoveAll(path); err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    err.Error(),
		}
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   fmt.Sprintf("Deleted %s", path),
	}
}

func (h *FileHandler) opMkdir(path string) StepResult {
	if err := os.MkdirAll(path, 0755); err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    err.Error(),
		}
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   fmt.Sprintf("Created directory %s", path),
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func errorIf(cond bool, msg string) string {
	if cond {
		return msg
	}
	return ""
}
