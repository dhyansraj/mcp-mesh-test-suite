package interpolate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/PaesslerAG/jsonpath"
)

// Context holds all variables available during interpolation
type Context struct {
	Config        map[string]any `json:"config"`          // Configuration values
	State         map[string]any `json:"state"`           // Shared state
	Captured      map[string]any `json:"captured"`        // Captured variables
	Last          map[string]any `json:"last"`            // Last step result (exit_code, stdout, stderr)
	Steps         map[string]any `json:"steps"`           // Step results by capture name
	Params        map[string]any `json:"params"`          // Routine parameters
	SuitePath     string         `json:"suite_path"`      // Suite directory path
	Workdir       string         `json:"workdir"`         // Working directory
	FixturesDir   string         `json:"fixtures_dir"`    // Fixtures directory
	Artifacts     string         `json:"artifacts"`       // Test-specific artifacts directory
	UCArtifacts   string         `json:"uc_artifacts"`    // Use-case level artifacts directory
	Extra         map[string]any `json:"-"`               // Additional top-level variables
}

// NewContext creates a new context with initialized maps
func NewContext() *Context {
	return &Context{
		Config:   make(map[string]any),
		State:    make(map[string]any),
		Captured: make(map[string]any),
		Last:     make(map[string]any),
		Steps:    make(map[string]any),
		Params:   make(map[string]any),
		Extra:    make(map[string]any),
	}
}

// Pattern for ${...} variables
var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Interpolate replaces all ${...} variables in a string with their values
func Interpolate(text string, ctx *Context) (string, error) {
	result := varPattern.ReplaceAllStringFunc(text, func(match string) string {
		// Extract variable name from ${varname}
		varName := match[2 : len(match)-1]
		value, err := ResolveVariable(varName, ctx)
		if err != nil || value == nil {
			return match // Keep original if not found
		}
		return fmt.Sprintf("%v", value)
	})
	return result, nil
}

// InterpolateMap recursively interpolates all string values in a map
func InterpolateMap(m map[string]any, ctx *Context) (map[string]any, error) {
	result := make(map[string]any)
	for k, v := range m {
		interpolatedKey, err := Interpolate(k, ctx)
		if err != nil {
			return nil, err
		}

		switch val := v.(type) {
		case string:
			interpolated, err := Interpolate(val, ctx)
			if err != nil {
				return nil, err
			}
			result[interpolatedKey] = interpolated
		case map[string]any:
			interpolated, err := InterpolateMap(val, ctx)
			if err != nil {
				return nil, err
			}
			result[interpolatedKey] = interpolated
		case []any:
			interpolated, err := InterpolateSlice(val, ctx)
			if err != nil {
				return nil, err
			}
			result[interpolatedKey] = interpolated
		default:
			result[interpolatedKey] = v
		}
	}
	return result, nil
}

// InterpolateSlice recursively interpolates all string values in a slice
func InterpolateSlice(s []any, ctx *Context) ([]any, error) {
	result := make([]any, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case string:
			interpolated, err := Interpolate(val, ctx)
			if err != nil {
				return nil, err
			}
			result[i] = interpolated
		case map[string]any:
			interpolated, err := InterpolateMap(val, ctx)
			if err != nil {
				return nil, err
			}
			result[i] = interpolated
		case []any:
			interpolated, err := InterpolateSlice(val, ctx)
			if err != nil {
				return nil, err
			}
			result[i] = interpolated
		default:
			result[i] = v
		}
	}
	return result, nil
}

// ResolveVariable resolves a variable reference to its value
// Supported formats:
// - config.packages.cli_version -> config value
// - state.agent_port -> state value
// - captured.output -> captured variable
// - steps.output.exit_code -> step result by capture name
// - last.exit_code -> last step result
// - json:$.path.to.field -> JSONPath on last.stdout
// - jq:.path.to.field -> jq query on last.stdout
// - jq:captured.varname:.path -> jq query on captured variable
// - jsonfile:/path:$.query -> JSONPath on file
// - file:/path/to/file -> File contents
// - fixture:expected/foo.json -> Fixture file contents
// - env:VAR_NAME -> Environment variable
// - params.name -> Routine parameter
func ResolveVariable(varName string, ctx *Context) (any, error) {
	// Handle prefixed variables
	switch {
	case strings.HasPrefix(varName, "config."):
		return resolvePath(ctx.Config, varName[7:]), nil

	case strings.HasPrefix(varName, "state."):
		return resolvePath(ctx.State, varName[6:]), nil

	case strings.HasPrefix(varName, "captured."):
		return resolvePath(ctx.Captured, varName[9:]), nil

	case strings.HasPrefix(varName, "last."):
		return resolvePath(ctx.Last, varName[5:]), nil

	case strings.HasPrefix(varName, "params."):
		return resolvePath(ctx.Params, varName[7:]), nil

	case strings.HasPrefix(varName, "steps."):
		return resolvePath(ctx.Steps, varName[6:]), nil

	case strings.HasPrefix(varName, "json:"):
		// JSONPath on stdout
		path := varName[5:]
		stdout, _ := ctx.Last["stdout"].(string)
		return resolveJSONPath(stdout, path)

	case strings.HasPrefix(varName, "jq:"):
		// jq query
		return resolveJQ(varName[3:], ctx)

	case strings.HasPrefix(varName, "jsonfile:"):
		// ${jsonfile:/path:$.query}
		return resolveJSONFile(varName[9:])

	case strings.HasPrefix(varName, "file:"):
		path := varName[5:]
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil
		}
		return string(data), nil

	case strings.HasPrefix(varName, "fixture:"):
		fixtureName := varName[8:]
		if ctx.FixturesDir != "" {
			path := filepath.Join(ctx.FixturesDir, fixtureName)
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, nil
			}
			return string(data), nil
		}
		return nil, nil

	case strings.HasPrefix(varName, "env:"):
		return os.Getenv(varName[4:]), nil
	}

	// Try common paths without prefix
	// Check last first (most common in assertions)
	if varName == "exit_code" || varName == "stdout" || varName == "stderr" {
		return ctx.Last[varName], nil
	}

	// Check top-level context variables
	switch varName {
	case "suite_path":
		return ctx.SuitePath, nil
	case "workdir":
		return ctx.Workdir, nil
	case "fixtures_dir":
		return ctx.FixturesDir, nil
	case "artifacts", "artifacts_path":
		return ctx.Artifacts, nil
	case "uc_artifacts", "uc_artifacts_path":
		return ctx.UCArtifacts, nil
	}

	// Check Extra map for additional top-level variables
	if val, ok := ctx.Extra[varName]; ok {
		return val, nil
	}

	// Then check captured
	if val, ok := ctx.Captured[varName]; ok {
		return val, nil
	}

	// Then check state
	if val, ok := ctx.State[varName]; ok {
		return val, nil
	}

	// Then check config (for unprefixed config access)
	return resolvePath(ctx.Config, varName), nil
}

// resolvePath resolves a dot-notation path in a map
func resolvePath(obj map[string]any, path string) any {
	if obj == nil {
		return nil
	}

	parts := strings.Split(path, ".")
	var value any = obj

	for _, part := range parts {
		if m, ok := value.(map[string]any); ok {
			value = m[part]
		} else {
			return nil
		}
	}
	return value
}

// resolveJSONPath executes a JSONPath query on JSON data
func resolveJSONPath(jsonStr string, path string) (any, error) {
	if jsonStr == "" {
		return nil, nil
	}

	var data any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, nil
	}

	result, err := jsonpath.Get(path, data)
	if err != nil {
		return nil, nil
	}
	return result, nil
}

// resolveJQ executes a jq query
func resolveJQ(query string, ctx *Context) (any, error) {
	var inputData string
	var jqQuery string

	// Check if querying a captured variable
	if strings.HasPrefix(query, "captured.") {
		rest := query[9:] // Remove "captured."
		if idx := strings.Index(rest, ":"); idx != -1 {
			varName := rest[:idx]
			jqQuery = rest[idx+1:]
			inputData, _ = ctx.Captured[varName].(string)
		} else {
			inputData, _ = ctx.Captured[rest].(string)
			jqQuery = "."
		}
	} else {
		jqQuery = query
		inputData, _ = ctx.Last["stdout"].(string)
	}

	if inputData == "" {
		return nil, nil
	}

	// Run jq command
	cmd := exec.Command("jq", "-r", jqQuery)
	cmd.Stdin = strings.NewReader(inputData)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, nil
	}

	output := strings.TrimSpace(stdout.String())

	// Try to parse as JSON
	if strings.HasPrefix(output, "{") || strings.HasPrefix(output, "[") {
		var result any
		if err := json.Unmarshal([]byte(output), &result); err == nil {
			return result, nil
		}
	}

	// Handle jq null output
	if output == "null" {
		return nil, nil
	}

	return output, nil
}

// resolveJSONFile handles ${jsonfile:/path:$.query}
func resolveJSONFile(spec string) (any, error) {
	var filePath, jpath string
	if idx := strings.Index(spec, ":"); idx != -1 {
		filePath = spec[:idx]
		jpath = spec[idx+1:]
	} else {
		filePath = spec
		jpath = "$"
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil
	}

	return resolveJSONPath(string(data), jpath)
}

// InterpolateWithGoTemplate uses Go's text/template after converting ${} syntax
// This is useful for more complex template operations
func InterpolateWithGoTemplate(text string, ctx *Context) (string, error) {
	// Convert ${x} to {{.x}}
	converted := convertSyntax(text)

	// Build template data
	data := buildTemplateData(ctx)

	// Create template with custom functions
	tmpl, err := template.New("").Funcs(templateFuncs()).Parse(converted)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execute error: %w", err)
	}

	return buf.String(), nil
}

// convertSyntax converts ${x.y.z} to {{.x.y.z}}
func convertSyntax(input string) string {
	return varPattern.ReplaceAllStringFunc(input, func(match string) string {
		varName := match[2 : len(match)-1]
		return "{{." + varName + "}}"
	})
}

// buildTemplateData creates a map suitable for Go templates
func buildTemplateData(ctx *Context) map[string]any {
	data := make(map[string]any)
	data["config"] = ctx.Config
	data["state"] = ctx.State
	data["captured"] = ctx.Captured
	data["last"] = ctx.Last
	data["steps"] = ctx.Steps
	data["params"] = ctx.Params
	data["suite_path"] = ctx.SuitePath
	data["workdir"] = ctx.Workdir
	data["fixtures_dir"] = ctx.FixturesDir
	data["artifacts"] = ctx.Artifacts
	data["artifacts_path"] = ctx.Artifacts // Alias for Python compatibility
	data["uc_artifacts"] = ctx.UCArtifacts
	data["uc_artifacts_path"] = ctx.UCArtifacts // Alias for Python compatibility

	// Merge extra vars
	for k, v := range ctx.Extra {
		data[k] = v
	}

	return data
}

// templateFuncs returns custom template functions
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"default": func(defaultVal, val any) any {
			if val == nil || val == "" {
				return defaultVal
			}
			return val
		},
		"env": func(name string) string {
			return os.Getenv(name)
		},
		"now": func() string {
			return time.Now().Format(time.RFC3339)
		},
		"json": func(v any) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
		"toInt": func(v any) int {
			switch val := v.(type) {
			case int:
				return val
			case float64:
				return int(val)
			case string:
				i, _ := strconv.Atoi(val)
				return i
			default:
				return 0
			}
		},
		"toString": func(v any) string {
			return fmt.Sprintf("%v", v)
		},
	}
}
