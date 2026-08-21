package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/textutil"
)

// ProbeHandler polls a shell command until it reports readiness, replacing the
// hand-rolled `for i in $(seq 1 40); do ... sleep 3; done` loops that test suites
// otherwise write inline.
//
// Step fields:
//
//	command           shell command to poll (same semantics as the shell handler)
//	interval          seconds (or "2s"/"1m") between attempts, default 2
//	timeout           overall deadline, default 60s
//	until             optional assertion expression evaluated against the latest
//	                  attempt's ${stdout}/${stderr}/${exit_code}; when absent the
//	                  success condition is exit code 0
//	success_threshold consecutive passes required, default 1
//	on_failure        optional shell command run once when the probe gives up
type ProbeHandler struct{}

const (
	defaultProbeInterval = 2 * time.Second
	defaultProbeTimeout  = 60 * time.Second

	// minProbeAttempt keeps a single attempt from being handed a zero or
	// negative timeout when the overall deadline is nearly spent.
	minProbeAttempt = 1 * time.Second

	// onFailureTimeout bounds the diagnostic command so a hanging `meshctl logs`
	// cannot outlive the probe it is explaining.
	onFailureTimeout = 30 * time.Second

	// maxProbeDetail bounds the per-attempt reason recorded in the trace.
	maxProbeDetail = 200
)

func (h *ProbeHandler) Name() string {
	return "probe"
}

func (h *ProbeHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	command, _ := step["command"].(string)
	if command == "" {
		return StepResult{
			Success: false,
			Error:   "probe handler requires 'command' field",
		}
	}

	interpolatedCmd, err := interpolate.Interpolate(command, ctx)
	if err != nil {
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("failed to interpolate command: %v", err),
		}
	}

	workdir := stepWorkdir(step, ctx)

	timeout, err := durationField(step, "timeout", defaultProbeTimeout)
	if err != nil {
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("probe handler: %v", err),
		}
	}

	interval, err := durationField(step, "interval", defaultProbeInterval)
	if err != nil {
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("probe handler: %v", err),
		}
	}

	threshold := parseCount(step["success_threshold"], 1)
	until := strings.TrimSpace(stringField(step, "until"))
	onFailure := stringField(step, "on_failure")
	label := probeLabel(step)

	start := time.Now()
	deadline := start.Add(timeout)
	trace := &probeTrace{start: start}

	var last StepResult
	attempts := 0
	consecutive := 0

	for {
		attempts++

		attemptTimeout := time.Until(deadline)
		if attemptTimeout < minProbeAttempt {
			attemptTimeout = minProbeAttempt
		}

		last = runShellCommand(interpolatedCmd, workdir, attemptTimeout)

		passed, detail := probeAttemptPassed(until, last, ctx)
		if passed {
			consecutive++
			if threshold > 1 {
				detail = fmt.Sprintf("%s [%d/%d]", detail, consecutive, threshold)
			}
		} else {
			consecutive = 0
		}
		trace.record(attempts, passed, detail)

		if consecutive >= threshold {
			return h.ready(label, trace, attempts, last)
		}

		// Stop once there is no room left for another attempt.
		if time.Until(deadline) < interval {
			break
		}
		time.Sleep(interval)
	}

	return h.gaveUp(label, trace, attempts, timeout, until, threshold, last, onFailure, workdir)
}

// ready builds the result for a probe that reached its success threshold.
// Stdout is the final attempt's output verbatim so `capture` behaves exactly as
// it does for the shell handler; the poll trace goes to stderr.
func (h *ProbeHandler) ready(label string, trace *probeTrace, attempts int, last StepResult) StepResult {
	var stderr strings.Builder
	stderr.WriteString(fmt.Sprintf("%s ready after %d attempt(s) in %s\n", label, attempts, trace.elapsed()))
	stderr.WriteString(trace.String())
	if s := strings.TrimRight(last.Stderr, "\n"); s != "" {
		stderr.WriteString("\n--- last attempt stderr ---\n")
		stderr.WriteString(s)
		stderr.WriteString("\n")
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   textutil.TruncateOutput(last.Stdout, textutil.MaxStepOutput),
		Stderr:   textutil.TruncateOutput(stderr.String(), textutil.MaxStepOutput),
	}
}

// gaveUp builds the result for a probe that hit its deadline. The trace leads
// stdout here (nothing captures a failed step, so readability wins) and the
// error carries the diagnostics that make the failure debuggable on its own.
func (h *ProbeHandler) gaveUp(
	label string,
	trace *probeTrace,
	attempts int,
	timeout time.Duration,
	until string,
	threshold int,
	last StepResult,
	onFailure string,
	workdir string,
) StepResult {
	condition := "exit code 0"
	if until != "" {
		condition = until
	}
	if threshold > 1 {
		condition = fmt.Sprintf("%s (x%d consecutive)", condition, threshold)
	}

	var stdout strings.Builder
	stdout.WriteString(fmt.Sprintf("%s NOT ready after %d attempt(s) in %s (waiting for: %s)\n",
		label, attempts, trace.elapsed(), condition))
	stdout.WriteString(trace.String())
	if s := strings.TrimRight(last.Stdout, "\n"); s != "" {
		stdout.WriteString("\n--- last attempt stdout ---\n")
		stdout.WriteString(s)
		stdout.WriteString("\n")
	}

	var stderr strings.Builder
	if s := strings.TrimRight(last.Stderr, "\n"); s != "" {
		stderr.WriteString("--- last attempt stderr ---\n")
		stderr.WriteString(s)
		stderr.WriteString("\n")
	}

	if strings.TrimSpace(onFailure) != "" {
		if stderr.Len() > 0 {
			stderr.WriteString("\n")
		}
		stderr.WriteString(runOnFailure(onFailure, workdir))
	}

	errMsg := fmt.Sprintf("%s did not become ready within %s (%d attempts, waiting for: %s); last attempt exit=%d",
		label, timeout, attempts, condition, last.ExitCode)
	if detail := strings.TrimSpace(last.Stdout); detail != "" {
		errMsg += "; stdout: " + detail
	}
	if detail := strings.TrimSpace(last.Stderr); detail != "" {
		errMsg += "; stderr: " + detail
	}
	if last.Error != "" {
		errMsg += "; " + last.Error
	}

	return StepResult{
		Success:  false,
		ExitCode: 1,
		Stdout:   textutil.TruncateOutput(stdout.String(), textutil.MaxStepOutput),
		Stderr:   textutil.TruncateOutput(stderr.String(), textutil.MaxStepOutput),
		Error:    textutil.Truncate(errMsg, textutil.MaxErrorDetail),
	}
}

// probeAttemptPassed decides whether one attempt satisfied the success
// condition and returns a short human-readable reason for the trace.
func probeAttemptPassed(until string, attempt StepResult, ctx *interpolate.Context) (bool, string) {
	if until == "" {
		if attempt.ExitCode == 0 {
			return true, "exit=0"
		}
		return false, fmt.Sprintf("exit=%d", attempt.ExitCode)
	}

	result := interpolate.EvaluateAssertion(until, probeContext(ctx, attempt))
	detail := strings.TrimSpace(result.Message)
	if detail == "" {
		detail = until
	}
	return result.Passed, textutil.Truncate(detail, maxProbeDetail)
}

// probeContext returns a scoped copy of the caller's context whose `last` holds
// the current attempt's output, so `until` expressions see ${stdout}, ${stderr},
// and ${exit_code} of the probe rather than of the preceding step.
//
// The copy is shallow: the maps it shares (config, state, captured, steps,
// params) are only read here, and `last` is replaced with a fresh map rather
// than mutated, so no probe internals leak into the caller's context or into
// later steps.
func probeContext(ctx *interpolate.Context, attempt StepResult) *interpolate.Context {
	scoped := *ctx
	scoped.Last = map[string]any{
		"exit_code": attempt.ExitCode,
		// Trailing whitespace is trimmed so `${stdout} == 3` works on the "3\n"
		// that jq, wc, and friends actually produce.
		"stdout": strings.TrimRight(attempt.Stdout, " \t\r\n"),
		"stderr": strings.TrimRight(attempt.Stderr, " \t\r\n"),
	}
	return &scoped
}

// runOnFailure runs the diagnostic command once and renders its output. Its own
// failure is reported inline and never replaces the probe's failure.
func runOnFailure(command, workdir string) string {
	result := runShellCommand(command, workdir, onFailureTimeout)

	var b strings.Builder
	b.WriteString("--- on_failure output ---\n")
	if s := strings.TrimRight(result.Stdout, "\n"); s != "" {
		b.WriteString(s)
		b.WriteString("\n")
	}
	if s := strings.TrimRight(result.Stderr, "\n"); s != "" {
		b.WriteString(s)
		b.WriteString("\n")
	}
	if !result.Success {
		detail := result.Error
		if detail == "" {
			detail = fmt.Sprintf("exit code %d", result.ExitCode)
		}
		b.WriteString(fmt.Sprintf("(on_failure command itself failed: %s)\n", detail))
	}
	return b.String()
}

func probeLabel(step map[string]any) string {
	if name := strings.TrimSpace(stringField(step, "name")); name != "" {
		return fmt.Sprintf("probe %q", name)
	}
	return "probe"
}

func stringField(step map[string]any, key string) string {
	s, _ := step[key].(string)
	return s
}

// probeTrace renders one line per attempt, collapsing runs of identical
// outcomes so a 150-attempt probe stays a few lines instead of a wall of text.
type probeTrace struct {
	start   time.Time
	lines   []string
	open    bool
	status  string
	first   int
	last    int
	firstAt time.Duration
	lastAt  time.Duration
}

func (t *probeTrace) record(attempt int, passed bool, detail string) {
	status := "fail"
	if passed {
		status = "pass"
	}
	if detail != "" {
		status = status + ": " + detail
	}

	at := time.Since(t.start)
	if t.open && t.status == status {
		t.last = attempt
		t.lastAt = at
		return
	}

	t.flush()
	t.open = true
	t.status = status
	t.first, t.last = attempt, attempt
	t.firstAt, t.lastAt = at, at
}

func (t *probeTrace) flush() {
	if !t.open {
		return
	}
	if t.first == t.last {
		t.lines = append(t.lines, fmt.Sprintf("  attempt %d (%s): %s",
			t.first, roundSeconds(t.firstAt), t.status))
	} else {
		t.lines = append(t.lines, fmt.Sprintf("  attempts %d-%d (%s-%s): %s (x%d)",
			t.first, t.last, roundSeconds(t.firstAt), roundSeconds(t.lastAt),
			t.status, t.last-t.first+1))
	}
	t.open = false
}

func (t *probeTrace) String() string {
	t.flush()
	if len(t.lines) == 0 {
		return ""
	}
	return strings.Join(t.lines, "\n") + "\n"
}

func (t *probeTrace) elapsed() string {
	return roundSeconds(time.Since(t.start))
}

func roundSeconds(d time.Duration) string {
	return d.Round(100 * time.Millisecond).String()
}
