package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// RunnerHandler extracts embedded runner binaries from the API server
type RunnerHandler struct{}

func (h *RunnerHandler) Name() string {
	return "runner"
}

func (h *RunnerHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	// Get required dest path
	dest, _ := step["dest"].(string)
	if dest == "" {
		return StepResult{
			Success: false,
			Error:   "runner handler requires 'dest' field",
		}
	}

	// Interpolate dest path
	dest, _ = interpolate.Interpolate(dest, ctx)

	// Make path absolute if not already
	if !filepath.IsAbs(dest) {
		workdir := ctx.Workdir
		if workdir == "" {
			workdir = "/workspace"
		}
		dest = filepath.Join(workdir, dest)
	}

	// Create dest directory
	if err := os.MkdirAll(dest, 0755); err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("failed to create directory %s: %v", dest, err),
		}
	}

	// Get API URL from environment
	apiURL := os.Getenv("TSUITE_API")
	if apiURL == "" {
		return StepResult{
			Success: false,
			Error:   "TSUITE_API environment variable not set",
		}
	}

	// List available runners
	runners, err := h.listRunners(apiURL)
	if err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("failed to list runners: %v", err),
		}
	}

	if len(runners) == 0 {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    "no runner binaries available. Build tsuite with: make build-with-runners",
		}
	}

	// Download each runner
	var extracted []string
	for _, name := range runners {
		destPath := filepath.Join(dest, name)
		if err := h.downloadRunner(apiURL, name, destPath); err != nil {
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Error:    fmt.Sprintf("failed to download %s: %v", name, err),
			}
		}

		// Make executable
		if err := os.Chmod(destPath, 0755); err != nil {
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Error:    fmt.Sprintf("failed to chmod %s: %v", name, err),
			}
		}

		extracted = append(extracted, name)
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   fmt.Sprintf("Extracted %d runner files to %s: %s", len(extracted), dest, strings.Join(extracted, ", ")),
	}
}

// listRunners fetches the list of available runner binaries from the API
func (h *RunnerHandler) listRunners(apiURL string) ([]string, error) {
	url := strings.TrimSuffix(apiURL, "/") + "/api/runners"

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Runners []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"runners"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var names []string
	for _, r := range result.Runners {
		names = append(names, r.Name)
	}
	return names, nil
}

// downloadRunner downloads a single runner binary from the API
func (h *RunnerHandler) downloadRunner(apiURL, name, destPath string) error {
	url := strings.TrimSuffix(apiURL, "/") + "/api/runners/" + name

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
