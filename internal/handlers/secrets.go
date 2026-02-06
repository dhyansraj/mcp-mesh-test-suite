package handlers

import (
	"bufio"
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

// SecretsHandler injects secrets into .env files
type SecretsHandler struct{}

func (h *SecretsHandler) Name() string {
	return "secrets"
}

func (h *SecretsHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	// Get required target path
	target, _ := step["target"].(string)
	if target == "" {
		return StepResult{
			Success: false,
			Error:   "secrets handler requires 'target' field",
		}
	}

	// Interpolate target path
	target, _ = interpolate.Interpolate(target, ctx)

	// Make path absolute if not already
	if !filepath.IsAbs(target) {
		workdir := ctx.Workdir
		if workdir == "" {
			workdir = "/workspace"
		}
		target = filepath.Join(workdir, target)
	}

	// Get optional source path
	source, _ := step["source"].(string)
	if source != "" {
		source, _ = interpolate.Interpolate(source, ctx)
		if !filepath.IsAbs(source) {
			workdir := ctx.Workdir
			if workdir == "" {
				workdir = "/workspace"
			}
			source = filepath.Join(workdir, source)
		}
	}

	// Get optional keys filter
	var keys []string
	if keysStr, ok := step["keys"].([]string); ok {
		keys = keysStr
	} else if keysRaw, ok := step["keys"].([]any); ok {
		for _, k := range keysRaw {
			if s, ok := k.(string); ok {
				keys = append(keys, s)
			}
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

	// Fetch secrets from API
	secrets, err := h.fetchSecrets(apiURL)
	if err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("failed to fetch secrets: %v", err),
		}
	}

	// Filter secrets by keys if specified
	if len(keys) > 0 {
		filtered := make(map[string]string)
		for _, key := range keys {
			if value, ok := secrets[key]; ok {
				filtered[key] = value
			}
		}
		secrets = filtered
	}

	// Create parent directories for target
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("failed to create directory: %v", err),
		}
	}

	// Build content
	var content strings.Builder

	// Copy source file content if provided
	if source != "" {
		sourceContent, err := os.ReadFile(source)
		if err != nil {
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Error:    fmt.Sprintf("failed to read source file: %v", err),
			}
		}
		content.Write(sourceContent)
		// Ensure newline at end
		if len(sourceContent) > 0 && sourceContent[len(sourceContent)-1] != '\n' {
			content.WriteString("\n")
		}
	}

	// Read existing target file to preserve non-secret entries
	existingKeys := make(map[string]bool)
	if _, err := os.Stat(target); err == nil {
		existingContent, err := os.ReadFile(target)
		if err == nil {
			scanner := bufio.NewScanner(strings.NewReader(string(existingContent)))
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					existingKeys[strings.TrimSpace(parts[0])] = true
				}
			}
		}
	}

	// Append secrets as KEY=value lines
	secretCount := 0
	for key, value := range secrets {
		content.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		secretCount++
	}

	// Write to target file
	if err := os.WriteFile(target, []byte(content.String()), 0600); err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("failed to write target file: %v", err),
		}
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   fmt.Sprintf("Wrote %d secrets to %s", secretCount, target),
	}
}

// fetchSecrets retrieves secrets from the API
func (h *SecretsHandler) fetchSecrets(apiURL string) (map[string]string, error) {
	url := strings.TrimSuffix(apiURL, "/") + "/api/secrets/values"

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var secrets map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&secrets); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return secrets, nil
}
