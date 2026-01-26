package handlers

import (
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
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
	r.Register(&FileHandler{})
	r.Register(&HTTPHandler{})
	r.Register(&NpmInstallHandler{})
	r.Register(&PipInstallHandler{})

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
