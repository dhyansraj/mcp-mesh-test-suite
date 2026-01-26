package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// HTTPHandler makes HTTP requests
type HTTPHandler struct{}

func (h *HTTPHandler) Name() string {
	return "http"
}

func (h *HTTPHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	method := "GET"
	if m, ok := step["method"].(string); ok && m != "" {
		method = strings.ToUpper(m)
	}

	url, _ := step["url"].(string)
	if url == "" {
		return StepResult{
			Success: false,
			Error:   "http handler requires 'url' field",
		}
	}

	// Interpolate URL
	url, _ = interpolate.Interpolate(url, ctx)

	timeout := 30
	if t, ok := step["timeout"].(int); ok && t > 0 {
		timeout = t
	}

	// Get headers
	headers := make(map[string]string)
	if h, ok := step["headers"].(map[string]any); ok {
		for k, v := range h {
			if vs, ok := v.(string); ok {
				vs, _ = interpolate.Interpolate(vs, ctx)
				headers[k] = vs
			}
		}
	}

	// Get body
	var bodyReader io.Reader
	if body, ok := step["body"]; ok {
		switch b := body.(type) {
		case string:
			bodyStr, _ := interpolate.Interpolate(b, ctx)
			bodyReader = strings.NewReader(bodyStr)
		case map[string]any:
			// Interpolate map values
			interpolatedMap, _ := interpolate.InterpolateMap(b, ctx)
			jsonBytes, err := json.Marshal(interpolatedMap)
			if err != nil {
				return StepResult{
					Success: false,
					Error:   fmt.Sprintf("failed to marshal body: %v", err),
				}
			}
			bodyReader = bytes.NewReader(jsonBytes)
			if _, ok := headers["Content-Type"]; !ok {
				headers["Content-Type"] = "application/json"
			}
		}
	}

	// Create request
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("failed to create request: %v", err),
		}
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Execute request
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("failed to read response: %v", err),
		}
	}

	success := resp.StatusCode < 400

	return StepResult{
		Success:  success,
		ExitCode: boolToInt(!success),
		Stdout:   string(body),
		Error:    errorIf(!success, fmt.Sprintf("HTTP %d", resp.StatusCode)),
	}
}
