package runner

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/config"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/handlers"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// runStepYAML walks a single step through the real pipeline a test.yaml step
// takes: YAML -> config.Step -> stepToMap -> InterpolateMap -> handler.
func runStepYAML(t *testing.T, stepYAML string, ctx *interpolate.Context) handlers.StepResult {
	t.Helper()

	var step config.Step
	if err := yaml.Unmarshal([]byte(stepYAML), &step); err != nil {
		t.Fatalf("unmarshalling step: %v", err)
	}

	stepMap, err := interpolate.InterpolateMap(stepToMap(step), ctx)
	if err != nil {
		t.Fatalf("interpolating step: %v", err)
	}

	return handlers.NewRegistry().Execute(step.Handler, stepMap, ctx)
}

// capturingServer records the first request it receives.
type capturingServer struct {
	*httptest.Server
	method  string
	headers http.Header
	body    string
}

func newCapturingServer(t *testing.T) *capturingServer {
	t.Helper()
	cs := &capturingServer{}
	cs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cs.method = r.Method
		cs.headers = r.Header.Clone()
		cs.body = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(cs.Close)
	return cs
}

func TestStepBodyAsMapIsSentAsJSON(t *testing.T) {
	srv := newCapturingServer(t)
	ctx := interpolate.NewContext()
	ctx.Captured["agent"] = "greeter"

	result := runStepYAML(t, `
handler: http
method: POST
url: `+srv.URL+`
body:
  name: ${captured.agent}
  capabilities:
    - greeting
`, ctx)

	if !result.Success {
		t.Fatalf("step failed: %s", result.Error)
	}
	if srv.method != http.MethodPost {
		t.Errorf("method = %q, want POST", srv.method)
	}
	if got := srv.headers.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(srv.body), &payload); err != nil {
		t.Fatalf("body %q is not JSON: %v", srv.body, err)
	}
	if payload["name"] != "greeter" {
		t.Errorf("body name = %v, want greeter (interpolated)", payload["name"])
	}
	caps, ok := payload["capabilities"].([]any)
	if !ok || len(caps) != 1 || caps[0] != "greeting" {
		t.Errorf("body capabilities = %v, want [greeting]", payload["capabilities"])
	}
}

func TestStepBodyAsStringIsSentVerbatim(t *testing.T) {
	srv := newCapturingServer(t)
	ctx := interpolate.NewContext()
	ctx.Captured["agent"] = "greeter"

	result := runStepYAML(t, `
handler: http
method: POST
url: `+srv.URL+`
body: '{"name": "${captured.agent}"}'
`, ctx)

	if !result.Success {
		t.Fatalf("step failed: %s", result.Error)
	}
	if want := `{"name": "greeter"}`; srv.body != want {
		t.Errorf("body = %q, want %q", srv.body, want)
	}
	// A raw string body must not get a Content-Type guessed for it.
	if got := srv.headers.Get("Content-Type"); got != "" {
		t.Errorf("Content-Type = %q, want empty for string body", got)
	}
}

func TestStepBodyMapRespectsExplicitContentType(t *testing.T) {
	srv := newCapturingServer(t)

	result := runStepYAML(t, `
handler: http
method: POST
url: `+srv.URL+`
headers:
  Content-Type: application/vnd.api+json
body:
  name: greeter
`, interpolate.NewContext())

	if !result.Success {
		t.Fatalf("step failed: %s", result.Error)
	}
	if got := srv.headers.Get("Content-Type"); got != "application/vnd.api+json" {
		t.Errorf("Content-Type = %q, want application/vnd.api+json", got)
	}
}

func TestStepHeadersReachHandlerInterpolated(t *testing.T) {
	srv := newCapturingServer(t)
	ctx := interpolate.NewContext()
	ctx.Captured["token"] = "s3cret"

	result := runStepYAML(t, `
handler: http
url: `+srv.URL+`
headers:
  Authorization: Bearer ${captured.token}
  X-Static: plain
`, ctx)

	if !result.Success {
		t.Fatalf("step failed: %s", result.Error)
	}
	if got := srv.headers.Get("Authorization"); got != "Bearer s3cret" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer s3cret")
	}
	if got := srv.headers.Get("X-Static"); got != "plain" {
		t.Errorf("X-Static = %q, want plain", got)
	}
}

func TestStepFileOperationWrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested", "config.yaml")

	ctx := interpolate.NewContext()
	ctx.Captured["level"] = "debug"

	result := runStepYAML(t, `
handler: file
operation: write
path: `+target+`
content: "log_level: ${captured.level}"
`, ctx)

	if !result.Success {
		t.Fatalf("step failed: %s", result.Error)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if want := "log_level: debug"; string(data) != want {
		t.Errorf("file content = %q, want %q", string(data), want)
	}
}

func TestStepWaitTypeHTTP(t *testing.T) {
	srv := newCapturingServer(t)

	result := runStepYAML(t, `
handler: wait
type: http
url: `+srv.URL+`
interval: 1
timeout: 5
`, interpolate.NewContext())

	if !result.Success {
		t.Fatalf("step failed: %s", result.Error)
	}
	if srv.method != http.MethodGet {
		t.Errorf("method = %q, want GET (wait type: http never polled)", srv.method)
	}
}

func TestStepToMapForwardsHandlerOptions(t *testing.T) {
	stepYAML := `
handler: maven-install
path: examples/java_agent
operation: write
type: http
packages:
  - mcp-mesh
  - pytest
replace_file_deps: false
strip_file_deps: false
strip_file_repos: false
align_version: false
m2_repo: /root/.m2/repository
`
	var step config.Step
	if err := yaml.Unmarshal([]byte(stepYAML), &step); err != nil {
		t.Fatalf("unmarshalling step: %v", err)
	}

	m := stepToMap(step)

	// Types must survive as YAML produced them: handlers assert on bool/[]any.
	for _, key := range []string{"replace_file_deps", "strip_file_deps", "strip_file_repos", "align_version"} {
		v, ok := m[key].(bool)
		if !ok {
			t.Errorf("%s = %#v, want bool", key, m[key])
			continue
		}
		if v {
			t.Errorf("%s = true, want false", key)
		}
	}
	if m["operation"] != "write" {
		t.Errorf("operation = %#v, want write", m["operation"])
	}
	if m["type"] != "http" {
		t.Errorf("type = %#v, want http", m["type"])
	}
	if m["m2_repo"] != "/root/.m2/repository" {
		t.Errorf("m2_repo = %#v, want /root/.m2/repository", m["m2_repo"])
	}
	pkgs, ok := m["packages"].([]any)
	if !ok || len(pkgs) != 2 {
		t.Fatalf("packages = %#v, want []any of len 2", m["packages"])
	}
	if pkgs[0] != "mcp-mesh" || pkgs[1] != "pytest" {
		t.Errorf("packages = %v, want [mcp-mesh pytest]", pkgs)
	}
	// Explicit struct fields still win over the inline catch-all.
	if m["path"] != "examples/java_agent" {
		t.Errorf("path = %#v, want examples/java_agent", m["path"])
	}
	if m["handler"] != "maven-install" {
		t.Errorf("handler = %#v, want maven-install", m["handler"])
	}
}

func TestLoadTestConfigWithStructuredBodyAndOptions(t *testing.T) {
	dir := t.TempDir()
	testYAML := `name: demo
test:
  - name: Register agent
    handler: http
    method: POST
    url: http://localhost:8000/register
    body:
      name: greeter
  - name: Write config
    handler: file
    operation: write
    path: out.txt
    content: hello
`
	if err := os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(testYAML), 0644); err != nil {
		t.Fatalf("writing test.yaml: %v", err)
	}

	cfg, err := config.LoadTestConfig(dir)
	if err != nil {
		t.Fatalf("LoadTestConfig() error = %v", err)
	}
	if len(cfg.Test) != 2 {
		t.Fatalf("len(Test) = %d, want 2", len(cfg.Test))
	}

	body, ok := stepToMap(cfg.Test[0])["body"].(map[string]any)
	if !ok {
		t.Fatalf("body = %#v, want map[string]any", stepToMap(cfg.Test[0])["body"])
	}
	if body["name"] != "greeter" {
		t.Errorf("body name = %#v, want greeter", body["name"])
	}
	if got := stepToMap(cfg.Test[1])["operation"]; got != "write" {
		t.Errorf("operation = %#v, want write", got)
	}
}

func TestStepToMapOmitsEmptyBody(t *testing.T) {
	var step config.Step
	if err := yaml.Unmarshal([]byte("handler: http\nurl: http://localhost\n"), &step); err != nil {
		t.Fatalf("unmarshalling step: %v", err)
	}
	if _, ok := stepToMap(step)["body"]; ok {
		t.Error("body present in step map, want omitted when unset")
	}
}
