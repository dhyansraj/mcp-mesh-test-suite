package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/textutil"
)

// StepResult holds the result of executing a step
type StepResult struct {
	Success  bool   `json:"success"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

// Handler is the interface for all step handlers
type Handler interface {
	// Name returns the handler name (e.g., "shell", "wait", "file")
	Name() string
	// Execute runs the handler with the given step configuration and context
	Execute(step map[string]any, ctx *interpolate.Context) StepResult
}

// Registry holds all registered handlers
type Registry struct {
	handlers map[string]Handler
}

// NewRegistry creates a new handler registry with all built-in handlers
func NewRegistry() *Registry {
	r := &Registry{
		handlers: make(map[string]Handler),
	}

	// Register built-in handlers
	r.Register(&ShellHandler{})
	r.Register(&WaitHandler{})
	r.Register(&ProbeHandler{})
	r.Register(&FileHandler{})
	r.Register(&HTTPHandler{})
	r.Register(&NpmInstallHandler{})
	r.Register(&PipInstallHandler{})
	r.Register(&MavenInstallHandler{})
	r.Register(&GradleInstallHandler{})
	r.Register(&SecretsHandler{})
	r.Register(&RunnerHandler{})

	return r
}

// Register adds a handler to the registry
func (r *Registry) Register(h Handler) {
	r.handlers[h.Name()] = h
}

// Get retrieves a handler by name
func (r *Registry) Get(name string) (Handler, bool) {
	h, ok := r.handlers[name]
	return h, ok
}

// Execute runs a step using the appropriate handler
func (r *Registry) Execute(handlerName string, step map[string]any, ctx *interpolate.Context) StepResult {
	handler, ok := r.Get(handlerName)
	if !ok {
		return StepResult{
			Success: false,
			Error:   "unknown handler: " + handlerName,
		}
	}

	return handler.Execute(step, ctx)
}

// commandFailureError builds a diagnostic error string for a command that exited
// non-zero, folding in the most useful output it produced so the message is not
// left empty when the failure detail only exists on stderr/stdout.
func commandFailureError(what string, exitCode int, stdout, stderr string) string {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = strings.TrimSpace(stdout)
	}
	if detail == "" {
		return fmt.Sprintf("%s exited with code %d (no output)", what, exitCode)
	}
	return fmt.Sprintf("%s exited with code %d: %s", what, exitCode, textutil.Truncate(detail, textutil.MaxErrorDetail))
}

// parseDuration normalizes a step's timeout/interval value. YAML hands back an
// int for `timeout: 300`, a float64 for `timeout: 2.5`, and a string for
// `timeout: "5m"`, so all three are accepted: bare numbers are seconds, strings
// are either a bare number of seconds or a Go duration ("300s", "5m").
// Anything missing, unparseable, or non-positive falls back to def.
func parseDuration(v any, def time.Duration) time.Duration {
	var d time.Duration

	switch val := v.(type) {
	case int:
		d = time.Duration(val) * time.Second
	case int64:
		d = time.Duration(val) * time.Second
	case float64:
		d = time.Duration(val * float64(time.Second))
	case time.Duration:
		d = val
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return def
		}
		if secs, err := strconv.ParseFloat(s, 64); err == nil {
			d = time.Duration(secs * float64(time.Second))
		} else if parsed, err := time.ParseDuration(s); err == nil {
			d = parsed
		} else {
			return def
		}
	default:
		return def
	}

	if d <= 0 {
		return def
	}
	return d
}

// parseCount normalizes a small positive integer step field, accepting the int,
// float64, and string forms YAML may produce. Non-positive or unparseable
// values fall back to def.
func parseCount(v any, def int) int {
	var n int

	switch val := v.(type) {
	case int:
		n = val
	case int64:
		n = int(val)
	case float64:
		n = int(val)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return def
		}
		n = parsed
	default:
		return def
	}

	if n <= 0 {
		return def
	}
	return n
}
