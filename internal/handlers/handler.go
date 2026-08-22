package handlers

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
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

// defaultInstallTimeout bounds a dependency resolution command (pip, npm, mvn,
// gradle) when the step does not set one.
const defaultInstallTimeout = 300 * time.Second

// durationField reads a duration-valued step option such as `timeout` or
// `interval`. Every handler goes through here so all of them accept the same
// forms, and so a value that is present but unusable fails the step instead of
// silently reverting to def: a typo'd `timeout: fivem` used to look exactly
// like no timeout at all.
//
// An absent (or nil) key is the only case that yields def.
func durationField(step map[string]any, key string, def time.Duration) (time.Duration, error) {
	v, ok := step[key]
	if !ok || v == nil {
		return def, nil
	}

	d, err := toDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid %s: must be positive, got %v", key, d)
	}
	return d, nil
}

// toDuration normalizes one duration value. YAML hands back an int for
// `timeout: 300`, a float64 for `timeout: 2.5`, and a string for
// `timeout: "5m"`, so all three are accepted: bare numbers are seconds, strings
// are either a bare number of seconds or a Go duration ("300s", "5m").
func toDuration(v any) (time.Duration, error) {
	switch val := v.(type) {
	case int:
		return time.Duration(val) * time.Second, nil
	case int64:
		return time.Duration(val) * time.Second, nil
	case float64:
		return time.Duration(val * float64(time.Second)), nil
	case time.Duration:
		return val, nil
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return 0, errors.New("value is empty")
		}
		if secs, err := strconv.ParseFloat(s, 64); err == nil {
			return time.Duration(secs * float64(time.Second)), nil
		}
		if parsed, err := time.ParseDuration(s); err == nil {
			return parsed, nil
		}
		return 0, fmt.Errorf("%q is neither a number of seconds nor a duration such as \"30s\" or \"5m\"", s)
	default:
		return 0, fmt.Errorf("%v (%T) is neither a number of seconds nor a duration string", v, v)
	}
}

// boolField reads a boolean step option such as `insecure_tls`. YAML hands back
// a bool for `insecure_tls: true`, but a value that arrived through
// interpolation is a string, so both forms are accepted. A value that is
// present but unusable fails the step instead of quietly reading as false:
// `insecure_tls: yes please` must not look exactly like not setting it.
//
// An absent (or nil) key is the only case that yields def.
func boolField(step map[string]any, key string, def bool) (bool, error) {
	v, ok := step[key]
	if !ok || v == nil {
		return def, nil
	}

	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(val))
		if err != nil {
			return false, fmt.Errorf("invalid %s: %q is not a boolean (use true or false)", key, val)
		}
		return b, nil
	default:
		return false, fmt.Errorf("invalid %s: %v (%T) is not a boolean", key, v, v)
	}
}

// tlsOptions are the per-step TLS knobs shared by the handlers that make
// outbound HTTP requests (`http`, and `wait` with `type: http`). Both are
// off by default, so a step that sets neither behaves exactly as it did before
// these options existed.
type tlsOptions struct {
	// insecure skips certificate verification entirely (`insecure_tls: true`).
	insecure bool
	// caCert is a path to a PEM bundle to trust in addition to the system
	// roots (`ca_cert:`), already interpolated by the caller.
	caCert string
}

func (o tlsOptions) set() bool { return o.insecure || o.caCert != "" }

// newHTTPClient builds the client a step should use.
//
// With neither TLS option set it returns a bare client on the default
// transport - identical to what the handlers built before, so the common case
// keeps http.DefaultTransport's connection pooling and proxy handling.
//
// insecure_tls and ca_cert are mutually exclusive rather than one taking
// precedence: "trust nothing" and "trust exactly this CA" are contradictory
// intents, and silently honoring one would hide the other.
func newHTTPClient(timeout time.Duration, opts tlsOptions) (*http.Client, error) {
	if !opts.set() {
		return &http.Client{Timeout: timeout}, nil
	}

	if opts.insecure && opts.caCert != "" {
		return nil, errors.New("insecure_tls and ca_cert are mutually exclusive: " +
			"insecure_tls skips certificate verification entirely, ca_cert asks for verification against a specific CA")
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if opts.insecure {
		// Explicit, documented, per-step opt-in.
		tlsConfig.InsecureSkipVerify = true //nolint:gosec // opt-in via insecure_tls
	} else {
		pool, err := caCertPool(opts.caCert)
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = pool
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig

	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

// caCertPool returns the system trust pool with the PEM bundle at path added to
// it. The extra CA is additive on purpose: a step that pins a private CA
// usually still needs to reach public-CA hosts, and replacing the pool would
// break that in a way that only shows up on the second request.
//
// A path that cannot be read, or a file with no usable certificate in it, is an
// error - falling back to default trust would turn a typo into a step that
// passes for the wrong reason.
func caCertPool(path string) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		// Not fatal: some platforms have no system pool to read. The bundle
		// below is then the only thing trusted, which is what was asked for.
		pool = x509.NewCertPool()
	}

	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ca_cert %s: %v", path, err)
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("ca_cert %s: no PEM certificate found in file", path)
	}

	return pool, nil
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
