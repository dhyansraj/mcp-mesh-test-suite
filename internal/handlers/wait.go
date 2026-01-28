package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// WaitHandler waits for a duration or condition
type WaitHandler struct{}

func (h *WaitHandler) Name() string {
	return "wait"
}

func (h *WaitHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	waitType := "seconds"
	if t, ok := step["type"].(string); ok && t != "" {
		waitType = t
	}

	switch waitType {
	case "seconds":
		return h.waitSeconds(step)
	case "http":
		return h.waitHTTP(step, ctx)
	default:
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("unknown wait type: %s", waitType),
		}
	}
}

func (h *WaitHandler) waitSeconds(step map[string]any) StepResult {
	seconds := 1
	if s, ok := step["seconds"].(int); ok && s > 0 {
		seconds = s
	}

	time.Sleep(time.Duration(seconds) * time.Second)

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   fmt.Sprintf("Waited %d seconds", seconds),
	}
}

func (h *WaitHandler) waitHTTP(step map[string]any, ctx *interpolate.Context) StepResult {
	url, _ := step["url"].(string)
	if url == "" {
		return StepResult{
			Success: false,
			Error:   "wait http requires 'url' field",
		}
	}

	// Interpolate URL
	url, _ = interpolate.Interpolate(url, ctx)

	timeout := 30
	if t, ok := step["timeout"].(int); ok && t > 0 {
		timeout = t
	}

	interval := 2
	if i, ok := step["interval"].(int); ok && i > 0 {
		interval = i
	}

	startTime := time.Now()
	timeoutDuration := time.Duration(timeout) * time.Second
	intervalDuration := time.Duration(interval) * time.Second

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for time.Since(startTime) < timeoutDuration {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 400 {
				return StepResult{
					Success:  true,
					ExitCode: 0,
					Stdout:   fmt.Sprintf("URL %s is ready (status %d)", url, resp.StatusCode),
				}
			}
		}
		time.Sleep(intervalDuration)
	}

	return StepResult{
		Success:  false,
		ExitCode: 1,
		Error:    fmt.Sprintf("URL %s not ready after %d seconds", url, timeout),
	}
}
