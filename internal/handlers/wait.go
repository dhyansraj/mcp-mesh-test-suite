package handlers

import (
	"fmt"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/textutil"
)

// WaitHandler waits for a duration or condition
type WaitHandler struct{}

const (
	defaultWaitSeconds     = 1 * time.Second
	defaultWaitHTTPTimeout = 30 * time.Second
	defaultWaitInterval    = 2 * time.Second
	// waitProbeTimeout bounds one poll, not the whole wait.
	waitProbeTimeout = 5 * time.Second
)

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
	duration, err := durationField(step, "seconds", defaultWaitSeconds)
	if err != nil {
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("wait handler: %v", err),
		}
	}

	time.Sleep(duration)

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   waitedMessage(duration),
	}
}

// waitedMessage keeps the historical "Waited 5 seconds" wording for whole-second
// waits, since that stdout is capturable and suites may match on it.
func waitedMessage(d time.Duration) string {
	if d%time.Second == 0 {
		return fmt.Sprintf("Waited %d seconds", int64(d/time.Second))
	}
	return fmt.Sprintf("Waited %s", d)
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

	timeout, err := durationField(step, "timeout", defaultWaitHTTPTimeout)
	if err != nil {
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("wait handler: %v", err),
		}
	}

	interval, err := durationField(step, "interval", defaultWaitInterval)
	if err != nil {
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("wait handler: %v", err),
		}
	}

	insecure, err := boolField(step, "insecure_tls", false)
	if err != nil {
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("wait handler: %v", err),
		}
	}

	caCert, _ := step["ca_cert"].(string)
	if caCert != "" {
		caCert, _ = interpolate.Interpolate(caCert, ctx)
	}

	client, err := newHTTPClient(waitProbeTimeout, tlsOptions{insecure: insecure, caCert: caCert})
	if err != nil {
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("wait handler: %v", err),
		}
	}

	startTime := time.Now()

	// The reason the last poll failed is the whole diagnostic value of a wait
	// that times out: a DNS failure, a refused connection, a rejected
	// certificate and a server stuck on 503 are otherwise indistinguishable.
	var (
		lastErr    error
		lastStatus int
	)

	for time.Since(startTime) < timeout {
		resp, err := client.Get(url)
		if err != nil {
			lastErr, lastStatus = err, 0
		} else {
			resp.Body.Close()
			if resp.StatusCode < 400 {
				return StepResult{
					Success:  true,
					ExitCode: 0,
					Stdout:   fmt.Sprintf("URL %s is ready (status %d)", url, resp.StatusCode),
				}
			}
			lastErr, lastStatus = nil, resp.StatusCode
		}
		time.Sleep(interval)
	}

	return StepResult{
		Success:  false,
		ExitCode: 1,
		Error:    waitNotReadyError(url, timeout, lastStatus, lastErr),
	}
}

// waitNotReadyError explains why the wait gave up, keeping the historical
// "URL X not ready after T" opening and appending whichever of the two possible
// last outcomes actually happened.
func waitNotReadyError(url string, timeout time.Duration, lastStatus int, lastErr error) string {
	base := fmt.Sprintf("URL %s not ready after %s", url, timeout)

	switch {
	case lastErr != nil:
		return fmt.Sprintf("%s: last attempt failed: %s", base,
			textutil.Truncate(lastErr.Error(), textutil.MaxErrorDetail))
	case lastStatus != 0:
		return fmt.Sprintf("%s: last response was HTTP %d (ready means a status below 400)", base, lastStatus)
	default:
		return base + ": no request completed before the deadline"
	}
}
