package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// These tests cover how each handler resolves its duration options. Every
// handler must accept the same forms - a bare number of seconds, a float, and a
// duration string - fall back to its default only when the option is absent,
// and fail the step when the option is present but unparseable.

func testCtx() *interpolate.Context {
	return interpolate.NewContext()
}

// wantError asserts the step failed with an error containing want.
func wantError(t *testing.T, result StepResult, want string) {
	t.Helper()
	if result.Success {
		t.Fatalf("step succeeded, want failure with error containing %q", want)
	}
	if !strings.Contains(result.Error, want) {
		t.Fatalf("error = %q, want it to contain %q", result.Error, want)
	}
}

// ---------------------------------------------------------------------------
// shell
// ---------------------------------------------------------------------------

func TestShellHandlerTimeoutForms(t *testing.T) {
	tests := []struct {
		name    string
		timeout any
		command string
		want    func(*testing.T, StepResult)
	}{
		{
			name:    "absent uses the default",
			command: "exit 0",
			want:    wantSuccess,
		},
		{
			name:    "duration string is honored",
			timeout: "5m",
			command: "exit 0",
			want:    wantSuccess,
		},
		{
			name:    "bare int is seconds",
			timeout: 1,
			command: "sleep 30",
			want:    wantTimedOut("command timed out after 1s"),
		},
		{
			name:    "sub-second duration string is honored",
			timeout: "150ms",
			command: "sleep 30",
			want:    wantTimedOut("command timed out after 150ms"),
		},
		{
			name:    "float is fractional seconds",
			timeout: 0.15,
			command: "sleep 30",
			want:    wantTimedOut("command timed out after 150ms"),
		},
		{
			name:    "typo is a step error",
			timeout: "fivem",
			command: "exit 0",
			want: func(t *testing.T, r StepResult) {
				wantError(t, r, `shell handler: invalid timeout: "fivem" is neither`)
			},
		},
		{
			name:    "zero is a step error",
			timeout: 0,
			command: "exit 0",
			want: func(t *testing.T, r StepResult) {
				wantError(t, r, "shell handler: invalid timeout: must be positive")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := map[string]any{"command": tt.command, "workdir": t.TempDir()}
			if tt.timeout != nil {
				step["timeout"] = tt.timeout
			}
			tt.want(t, (&ShellHandler{}).Execute(step, testCtx()))
		})
	}
}

func wantSuccess(t *testing.T, r StepResult) {
	t.Helper()
	if !r.Success {
		t.Fatalf("step failed: %s", r.Error)
	}
}

func wantTimedOut(want string) func(*testing.T, StepResult) {
	return func(t *testing.T, r StepResult) {
		t.Helper()
		if r.ExitCode != 124 {
			t.Fatalf("exit code = %d (error %q), want 124 (timed out)", r.ExitCode, r.Error)
		}
		if !strings.Contains(r.Error, want) {
			t.Errorf("error = %q, want it to contain %q", r.Error, want)
		}
	}
}

// TestShellHandlerRejectsBadTimeoutBeforeRunning is the regression this whole
// change is about: an unparseable timeout used to be indistinguishable from no
// timeout, so the command ran anyway under the default.
func TestShellHandlerRejectsBadTimeoutBeforeRunning(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")

	result := (&ShellHandler{}).Execute(map[string]any{
		"command": "touch " + marker,
		"workdir": dir,
		"timeout": "fivem",
	}, testCtx())

	wantError(t, result, "invalid timeout")
	if _, err := os.Stat(marker); err == nil {
		t.Error("command ran despite the invalid timeout; the step should have failed first")
	}
}

// ---------------------------------------------------------------------------
// probe
// ---------------------------------------------------------------------------

func TestProbeHandlerDurationForms(t *testing.T) {
	tests := []struct {
		name     string
		timeout  any
		interval any
		command  string
		want     func(*testing.T, StepResult)
	}{
		{
			name:    "absent uses the defaults",
			command: "exit 0",
			want:    wantSuccess,
		},
		{
			name:     "duration strings are honored",
			timeout:  "200ms",
			interval: "50ms",
			command:  "exit 1",
			want:     wantGaveUp("did not become ready within 200ms"),
		},
		{
			name:     "bare int is seconds",
			timeout:  1,
			interval: 1,
			command:  "exit 1",
			want:     wantGaveUp("did not become ready within 1s"),
		},
		{
			name:     "float is fractional seconds",
			timeout:  0.2,
			interval: 0.05,
			command:  "exit 1",
			want:     wantGaveUp("did not become ready within 200ms"),
		},
		{
			name:    "typo'd timeout is a step error",
			timeout: "fivem",
			command: "exit 0",
			want: func(t *testing.T, r StepResult) {
				wantError(t, r, `probe handler: invalid timeout: "fivem" is neither`)
			},
		},
		{
			name:     "typo'd interval is a step error",
			interval: "every 2s",
			command:  "exit 0",
			want: func(t *testing.T, r StepResult) {
				wantError(t, r, `probe handler: invalid interval: "every 2s" is neither`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := map[string]any{"command": tt.command, "workdir": t.TempDir()}
			if tt.timeout != nil {
				step["timeout"] = tt.timeout
			}
			if tt.interval != nil {
				step["interval"] = tt.interval
			}
			tt.want(t, (&ProbeHandler{}).Execute(step, testCtx()))
		})
	}
}

func wantGaveUp(want string) func(*testing.T, StepResult) {
	return func(t *testing.T, r StepResult) {
		t.Helper()
		if r.Success {
			t.Fatalf("probe succeeded, want it to give up with %q", want)
		}
		if !strings.Contains(r.Error, want) {
			t.Errorf("error = %q, want it to contain %q", r.Error, want)
		}
	}
}

// ---------------------------------------------------------------------------
// wait (type: seconds)
// ---------------------------------------------------------------------------

func TestWaitHandlerSecondsForms(t *testing.T) {
	tests := []struct {
		name    string
		seconds any
		want    func(*testing.T, StepResult, time.Duration)
	}{
		{
			name: "absent waits the default second",
			want: waitedFor(1*time.Second, "Waited 1 seconds"),
		},
		{
			name:    "bare int is seconds",
			seconds: 1,
			want:    waitedFor(1*time.Second, "Waited 1 seconds"),
		},
		{
			name:    "duration string is honored",
			seconds: "20ms",
			want:    waitedFor(20*time.Millisecond, "Waited 20ms"),
		},
		{
			name:    "float is fractional seconds",
			seconds: 0.05,
			want:    waitedFor(50*time.Millisecond, "Waited 50ms"),
		},
		{
			name:    "typo is a step error",
			seconds: "a while",
			want: func(t *testing.T, r StepResult, _ time.Duration) {
				wantError(t, r, `wait handler: invalid seconds: "a while" is neither`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := map[string]any{"type": "seconds"}
			if tt.seconds != nil {
				step["seconds"] = tt.seconds
			}
			start := time.Now()
			result := (&WaitHandler{}).Execute(step, testCtx())
			tt.want(t, result, time.Since(start))
		})
	}
}

func waitedFor(want time.Duration, stdout string) func(*testing.T, StepResult, time.Duration) {
	return func(t *testing.T, r StepResult, elapsed time.Duration) {
		t.Helper()
		wantSuccess(t, r)
		if r.Stdout != stdout {
			t.Errorf("stdout = %q, want %q", r.Stdout, stdout)
		}
		if elapsed < want {
			t.Errorf("returned after %v, want at least %v", elapsed, want)
		}
		// A wait must not silently fall back to a longer default.
		if elapsed > want+5*time.Second {
			t.Errorf("returned after %v, want roughly %v", elapsed, want)
		}
	}
}

// ---------------------------------------------------------------------------
// wait (type: http)
// ---------------------------------------------------------------------------

func TestWaitHandlerHTTPDurationForms(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		timeout  any
		interval any
		want     func(*testing.T, StepResult, int32)
	}{
		{
			name:   "absent uses the defaults",
			status: http.StatusOK,
			want: func(t *testing.T, r StepResult, hits int32) {
				wantSuccess(t, r)
				if hits == 0 {
					t.Error("no request was made")
				}
			},
		},
		{
			name:     "duration strings are honored",
			status:   http.StatusInternalServerError,
			timeout:  "200ms",
			interval: "50ms",
			want: func(t *testing.T, r StepResult, hits int32) {
				wantError(t, r, "not ready after 200ms")
				if hits < 2 {
					t.Errorf("made %d requests in 200ms at a 50ms interval, want at least 2", hits)
				}
			},
		},
		{
			name:     "bare int is seconds",
			status:   http.StatusInternalServerError,
			timeout:  1,
			interval: 1,
			want: func(t *testing.T, r StepResult, _ int32) {
				wantError(t, r, "not ready after 1s")
			},
		},
		{
			name:     "float is fractional seconds",
			status:   http.StatusInternalServerError,
			timeout:  0.2,
			interval: 0.05,
			want: func(t *testing.T, r StepResult, _ int32) {
				wantError(t, r, "not ready after 200ms")
			},
		},
		{
			name:    "typo'd timeout is a step error",
			status:  http.StatusOK,
			timeout: "fivem",
			want: func(t *testing.T, r StepResult, hits int32) {
				wantError(t, r, `wait handler: invalid timeout: "fivem" is neither`)
				if hits != 0 {
					t.Errorf("made %d requests despite the invalid timeout", hits)
				}
			},
		},
		{
			name:     "typo'd interval is a step error",
			status:   http.StatusOK,
			interval: "often",
			want: func(t *testing.T, r StepResult, hits int32) {
				wantError(t, r, `wait handler: invalid interval: "often" is neither`)
				if hits != 0 {
					t.Errorf("made %d requests despite the invalid interval", hits)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&hits, 1)
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			step := map[string]any{"type": "http", "url": srv.URL}
			if tt.timeout != nil {
				step["timeout"] = tt.timeout
			}
			if tt.interval != nil {
				step["interval"] = tt.interval
			}

			result := (&WaitHandler{}).Execute(step, testCtx())
			tt.want(t, result, atomic.LoadInt32(&hits))
		})
	}
}

// ---------------------------------------------------------------------------
// http
// ---------------------------------------------------------------------------

func TestHTTPHandlerTimeoutForms(t *testing.T) {
	// The server is slower than the short timeouts below but well inside the
	// one-second and default ones, so each form's magnitude is observable.
	const serverDelay = 300 * time.Millisecond

	tests := []struct {
		name    string
		timeout any
		want    func(*testing.T, StepResult, int32)
	}{
		{
			name: "absent uses the default",
			want: func(t *testing.T, r StepResult, _ int32) { wantSuccess(t, r) },
		},
		{
			name:    "duration string is honored",
			timeout: "5m",
			want:    func(t *testing.T, r StepResult, _ int32) { wantSuccess(t, r) },
		},
		{
			name:    "bare int is seconds",
			timeout: 1,
			want:    func(t *testing.T, r StepResult, _ int32) { wantSuccess(t, r) },
		},
		{
			name:    "sub-second duration string is honored",
			timeout: "50ms",
			want: func(t *testing.T, r StepResult, _ int32) {
				wantError(t, r, "request failed")
			},
		},
		{
			name:    "float is fractional seconds",
			timeout: 0.05,
			want: func(t *testing.T, r StepResult, _ int32) {
				wantError(t, r, "request failed")
			},
		},
		{
			name:    "typo is a step error",
			timeout: "fivem",
			want: func(t *testing.T, r StepResult, hits int32) {
				wantError(t, r, `http handler: invalid timeout: "fivem" is neither`)
				if hits != 0 {
					t.Errorf("made %d requests despite the invalid timeout", hits)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&hits, 1)
				time.Sleep(serverDelay)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			step := map[string]any{"url": srv.URL}
			if tt.timeout != nil {
				step["timeout"] = tt.timeout
			}

			result := (&HTTPHandler{}).Execute(step, testCtx())
			tt.want(t, result, atomic.LoadInt32(&hits))
		})
	}
}

// ---------------------------------------------------------------------------
// pip-install / npm-install / maven-install / gradle-install
// ---------------------------------------------------------------------------

// installHandler describes one dependency-install handler well enough to run
// the shared timeout table against it: a path that does not exist (so a valid
// timeout gets past parsing and stops at the missing project) and a minimal
// real project (so an already-expired timeout proves the value is applied).
type installHandler struct {
	name        string
	handler     Handler
	notFound    string
	timedOut    string
	makeProject func(t *testing.T) string
}

func installHandlers() []installHandler {
	return []installHandler{
		{
			name:     "pip-install",
			handler:  &PipInstallHandler{},
			notFound: "requirements.txt not found",
			timedOut: "pip install timed out",
			makeProject: writeProject(map[string]string{
				"requirements.txt": "requests\n",
			}),
		},
		{
			name:     "npm-install",
			handler:  &NpmInstallHandler{},
			notFound: "package.json not found",
			timedOut: "npm install timed out",
			makeProject: writeProject(map[string]string{
				"package.json": `{"name":"t","version":"1.0.0"}`,
			}),
		},
		{
			name:     "maven-install",
			handler:  &MavenInstallHandler{},
			notFound: "pom.xml not found",
			timedOut: "mvn dependency:resolve timed out",
			makeProject: writeProject(map[string]string{
				"pom.xml": "<project></project>",
			}),
		},
		{
			name:     "gradle-install",
			handler:  &GradleInstallHandler{},
			notFound: "neither build.gradle nor build.gradle.kts found",
			timedOut: "gradle dependencies timed out",
			makeProject: writeProject(map[string]string{
				"build.gradle": "plugins { id 'java' }\n",
			}),
		},
	}
}

func writeProject(files map[string]string) func(*testing.T) string {
	return func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatalf("writing %s: %v", name, err)
			}
		}
		return dir
	}
}

// TestInstallHandlerTimeoutForms checks that each install handler accepts every
// duration form and rejects a malformed one. The project path does not exist,
// so an accepted timeout surfaces as the handler's own "not found" error - which
// is exactly the signal that parsing got out of the way.
func TestInstallHandlerTimeoutForms(t *testing.T) {
	forms := []struct {
		name    string
		timeout any
		wantErr string // empty => expect the handler's notFound error
	}{
		{name: "absent uses the default", timeout: nil},
		{name: "bare int is seconds", timeout: 600},
		{name: "duration string is honored", timeout: "10m"},
		{name: "float is fractional seconds", timeout: 2.5},
		{name: "numeric string is seconds", timeout: "600"},
		{name: "typo is a step error", timeout: "tenminutes", wantErr: `invalid timeout: "tenminutes" is neither`},
		{name: "zero is a step error", timeout: 0, wantErr: "invalid timeout: must be positive"},
		{name: "wrong type is a step error", timeout: true, wantErr: "invalid timeout: true (bool) is neither"},
	}

	for _, ih := range installHandlers() {
		t.Run(ih.name, func(t *testing.T) {
			for _, form := range forms {
				t.Run(form.name, func(t *testing.T) {
					step := map[string]any{"path": filepath.Join(t.TempDir(), "no-such-project")}
					if form.timeout != nil {
						step["timeout"] = form.timeout
					}

					result := ih.handler.Execute(step, testCtx())

					want := ih.notFound
					if form.wantErr != "" {
						want = ih.name + " handler: " + form.wantErr
					}
					wantError(t, result, want)
				})
			}
		})
	}
}

// TestInstallHandlerTimeoutIsApplied proves the parsed duration reaches the
// command context rather than just being accepted: an already-expired timeout
// makes every install handler report its own timeout error.
func TestInstallHandlerTimeoutIsApplied(t *testing.T) {
	for _, ih := range installHandlers() {
		t.Run(ih.name, func(t *testing.T) {
			for _, timeout := range []any{"1ns", 1e-9} {
				result := ih.handler.Execute(map[string]any{
					"path":    ih.makeProject(t),
					"timeout": timeout,
				}, testCtx())

				if result.ExitCode != 124 {
					t.Fatalf("timeout %v: exit code = %d (error %q), want 124", timeout, result.ExitCode, result.Error)
				}
				wantError(t, result, ih.timedOut)
			}
		})
	}
}

// TestNpmInstallRejectsBadTimeoutBeforeRewritingPackageJSON checks the malformed
// step is caught before the handler starts editing the project on disk.
func TestNpmInstallRejectsBadTimeoutBeforeRewritingPackageJSON(t *testing.T) {
	dir := t.TempDir()
	packageJSON := filepath.Join(dir, "package.json")
	original := `{"name":"t","version":"1.0.0","dependencies":{"@mcpmesh/sdk":"file:../sdk"}}`
	if err := os.WriteFile(packageJSON, []byte(original), 0o644); err != nil {
		t.Fatalf("writing package.json: %v", err)
	}

	result := (&NpmInstallHandler{}).Execute(map[string]any{
		"path":    dir,
		"timeout": "fivem",
	}, testCtx())

	wantError(t, result, "invalid timeout")

	after, err := os.ReadFile(packageJSON)
	if err != nil {
		t.Fatalf("reading package.json: %v", err)
	}
	if string(after) != original {
		t.Errorf("package.json was rewritten despite the invalid timeout:\n%s", after)
	}
}
