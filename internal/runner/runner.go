package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/config"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/handlers"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// TestRunner executes tests locally (inside container or standalone)
type TestRunner struct {
	suitePath      string
	suiteConfig    *config.SuiteConfig
	globalRoutines map[string]config.RoutineDefinition
	ucRoutines     map[string]config.RoutineDefinition // UC-level routines
	handlers       *handlers.Registry
	serverURL      string
	runID          string
	baseWorkdir    string // Base workdir for standalone mode
}

// TestResult holds the complete result of a test execution
type TestResult struct {
	TestID     string
	TestName   string
	Passed     bool
	Error      string
	Duration   time.Duration
	Steps      []StepResult
	Assertions []AssertionResult
}

// StepResult holds the result of a single step
type StepResult struct {
	Phase    string // "pre_run", "test", "post_run"
	Index    int
	Name     string
	Handler  string
	Success  bool
	ExitCode int
	Stdout   string
	Stderr   string
	Error    string
}

// AssertionResult holds the result of an assertion
type AssertionResult struct {
	Index    int
	Expr     string
	Message  string
	Passed   bool
	Details  string
	Actual   string
	Expected string
}

// NewTestRunner creates a new test runner
func NewTestRunner(suitePath string, serverURL string, runID string, baseWorkdir string) (*TestRunner, error) {
	// Load suite config
	suiteConfig, err := config.LoadSuiteConfig(suitePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load suite config: %w", err)
	}

	// Load global routines
	globalRoutinesConfig, err := config.LoadGlobalRoutines(suitePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load global routines: %w", err)
	}

	return &TestRunner{
		suitePath:      suitePath,
		suiteConfig:    suiteConfig,
		globalRoutines: globalRoutinesConfig.Routines,
		ucRoutines:     make(map[string]config.RoutineDefinition),
		handlers:       handlers.NewRegistry(),
		serverURL:      serverURL,
		runID:          runID,
		baseWorkdir:    baseWorkdir,
	}, nil
}

// RunTest executes a single test
func (r *TestRunner) RunTest(testID string) (*TestResult, error) {
	startTime := time.Now()

	// Parse test path
	parts := strings.Split(testID, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid test ID format: %s (expected uc/tc)", testID)
	}
	ucName := parts[0]
	tcName := parts[1]

	testPath := filepath.Join(r.suitePath, "suites", ucName, tcName)

	// Load test config
	testConfig, err := config.LoadTestConfig(testPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load test config: %w", err)
	}

	// Load UC-level routines
	ucPath := filepath.Join(r.suitePath, "suites", ucName)
	ucRoutinesConfig, err := config.LoadUseCaseRoutines(ucPath)
	if err == nil {
		r.ucRoutines = ucRoutinesConfig.Routines
	}

	// Determine workdir based on mode
	var workdir string
	mode := r.suiteConfig.Suite.Mode
	if mode == "docker" {
		// Docker mode uses /workspace inside container
		workdir = "/workspace"
	} else {
		// Standalone mode uses temp directory
		if r.baseWorkdir != "" {
			// Create test-specific workdir under base
			workdir = filepath.Join(r.baseWorkdir, strings.ReplaceAll(testID, "/", "_"))
			if err := os.MkdirAll(workdir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create workdir: %w", err)
			}
		} else {
			// Fallback to suite path if no base workdir
			workdir = r.suitePath
		}
	}

	// Build execution context
	ctx := interpolate.NewContext()
	ctx.Config = r.suiteConfig.ToMap()
	ctx.SuitePath = r.suitePath
	ctx.Workdir = workdir
	ctx.FixturesDir = filepath.Join(r.suitePath, "fixtures")
	ctx.Artifacts = filepath.Join(r.suitePath, "suites", testID, "artifacts")
	ctx.UCArtifacts = filepath.Join(r.suitePath, "suites", ucName, "artifacts")
	ctx.Extra["test_id"] = testID
	ctx.Extra["uc_name"] = ucName
	ctx.Extra["tc_name"] = tcName

	result := &TestResult{
		TestID:   testID,
		TestName: testConfig.Name,
		Passed:   true,
		Steps:    []StepResult{},
	}

	// Execute pre_run
	for i, step := range testConfig.PreRun {
		stepResult := r.executeStep(step, ctx, "pre_run", i)
		result.Steps = append(result.Steps, stepResult)

		if !stepResult.Success && !step.IgnoreErrors {
			result.Passed = false
			result.Error = fmt.Sprintf("pre_run step %d failed: %s", i, stepResult.Error)
			break
		}

		// Update context
		r.updateContext(ctx, stepResult, step)
	}

	// Execute test steps (if pre_run succeeded)
	if result.Passed {
		for i, step := range testConfig.Test {
			stepResult := r.executeStep(step, ctx, "test", i)
			result.Steps = append(result.Steps, stepResult)

			if !stepResult.Success && !step.IgnoreErrors {
				result.Passed = false
				result.Error = fmt.Sprintf("test step %d failed: %s", i, stepResult.Error)
				break
			}

			// Update context
			r.updateContext(ctx, stepResult, step)
		}
	}

	// Evaluate assertions (if test steps succeeded)
	if result.Passed {
		for i, assertion := range testConfig.Assertions {
			assertResult := interpolate.EvaluateAssertion(assertion.Expr, ctx)

			result.Assertions = append(result.Assertions, AssertionResult{
				Index:    i,
				Expr:     assertion.Expr,
				Message:  assertion.Message,
				Passed:   assertResult.Passed,
				Details:  assertResult.Message,
				Actual:   assertResult.ActualValue,
				Expected: assertResult.ExpectedValue,
			})

			if !assertResult.Passed {
				result.Passed = false
			}
		}
	}

	// Execute post_run (always)
	for i, step := range testConfig.PostRun {
		step.IgnoreErrors = true // Always ignore errors in post_run
		stepResult := r.executeStep(step, ctx, "post_run", i)
		result.Steps = append(result.Steps, stepResult)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// executeStep runs a single step
func (r *TestRunner) executeStep(step config.Step, ctx *interpolate.Context, phase string, index int) StepResult {
	// Check if this is a routine call
	if step.Routine != "" {
		return r.executeRoutine(step, ctx, phase, index)
	}

	// Execute handler
	handlerName := step.Handler
	if handlerName == "" {
		return StepResult{
			Phase:   phase,
			Index:   index,
			Name:    step.Name,
			Handler: "",
			Success: false,
			Error:   "step missing 'handler' or 'routine'",
		}
	}

	// Convert step to map for handler
	stepMap := stepToMap(step)

	// Interpolate step values
	interpolatedMap, err := interpolate.InterpolateMap(stepMap, ctx)
	if err != nil {
		return StepResult{
			Phase:   phase,
			Index:   index,
			Name:    step.Name,
			Handler: handlerName,
			Success: false,
			Error:   fmt.Sprintf("interpolation failed: %v", err),
		}
	}

	// Execute handler
	handlerResult := r.handlers.Execute(handlerName, interpolatedMap, ctx)

	return StepResult{
		Phase:    phase,
		Index:    index,
		Name:     step.Name,
		Handler:  handlerName,
		Success:  handlerResult.Success,
		ExitCode: handlerResult.ExitCode,
		Stdout:   handlerResult.Stdout,
		Stderr:   handlerResult.Stderr,
		Error:    handlerResult.Error,
	}
}

// executeRoutine runs a routine
func (r *TestRunner) executeRoutine(step config.Step, ctx *interpolate.Context, phase string, index int) StepResult {
	routineRef := step.Routine
	params := step.Params

	// Resolve routine name
	var routine *config.RoutineDefinition
	if strings.HasPrefix(routineRef, "global.") {
		routineName := routineRef[7:]
		if rd, ok := r.globalRoutines[routineName]; ok {
			routine = &rd
		}
	} else {
		// Try UC-level routines first, then global
		if rd, ok := r.ucRoutines[routineRef]; ok {
			routine = &rd
		} else if rd, ok := r.globalRoutines[routineRef]; ok {
			routine = &rd
		}
	}

	if routine == nil {
		return StepResult{
			Phase:   phase,
			Index:   index,
			Name:    step.Name,
			Handler: routineRef,
			Success: false,
			Error:   fmt.Sprintf("routine not found: %s", routineRef),
		}
	}

	// Interpolate params
	interpolatedParams := make(map[string]any)
	for k, v := range params {
		if s, ok := v.(string); ok {
			interpolated, _ := interpolate.Interpolate(s, ctx)
			interpolatedParams[k] = interpolated
		} else {
			interpolatedParams[k] = v
		}
	}

	// Create routine context with params
	routineCtx := *ctx // shallow copy
	routineCtx.Params = interpolatedParams

	// Execute routine steps
	for i, routineStep := range routine.Steps {
		stepResult := r.executeStep(routineStep, &routineCtx, phase, i)

		if !stepResult.Success && !routineStep.IgnoreErrors {
			return StepResult{
				Phase:    phase,
				Index:    index,
				Name:     step.Name,
				Handler:  routineRef,
				Success:  false,
				ExitCode: stepResult.ExitCode,
				Stdout:   stepResult.Stdout,
				Stderr:   stepResult.Stderr,
				Error:    fmt.Sprintf("routine step %d failed: %s", i, stepResult.Error),
			}
		}

		// Update routine context
		r.updateContext(&routineCtx, stepResult, routineStep)

		// Copy captured values back to main context
		for k, v := range routineCtx.Captured {
			ctx.Captured[k] = v
		}
		for k, v := range routineCtx.Steps {
			ctx.Steps[k] = v
		}
	}

	return StepResult{
		Phase:   phase,
		Index:   index,
		Name:    step.Name,
		Handler: routineRef,
		Success: true,
	}
}

// updateContext updates the execution context after a step
func (r *TestRunner) updateContext(ctx *interpolate.Context, result StepResult, step config.Step) {
	// Update last
	ctx.Last = map[string]any{
		"exit_code": result.ExitCode,
		"stdout":    result.Stdout,
		"stderr":    result.Stderr,
	}

	// Handle capture
	if step.Capture != "" {
		// Store full step result
		ctx.Steps[step.Capture] = map[string]any{
			"exit_code": result.ExitCode,
			"stdout":    result.Stdout,
			"stderr":    result.Stderr,
			"success":   result.Success,
			"error":     result.Error,
		}

		// Store stdout in captured for backward compatibility
		if result.Success {
			ctx.Captured[step.Capture] = result.Stdout
		}
	}
}

// stepToMap converts a Step struct to a map for handler execution
func stepToMap(step config.Step) map[string]any {
	m := make(map[string]any)

	if step.Name != "" {
		m["name"] = step.Name
	}
	if step.Handler != "" {
		m["handler"] = step.Handler
	}
	if step.Command != "" {
		m["command"] = step.Command
	}
	if step.Workdir != "" {
		m["workdir"] = step.Workdir
	}
	if step.Capture != "" {
		m["capture"] = step.Capture
	}
	if step.Timeout > 0 {
		m["timeout"] = step.Timeout
	}
	if step.IgnoreErrors {
		m["ignore_errors"] = step.IgnoreErrors
	}
	if step.Path != "" {
		m["path"] = step.Path
	}
	if step.Seconds > 0 {
		m["seconds"] = step.Seconds
	}
	if step.URL != "" {
		m["url"] = step.URL
	}
	if step.Method != "" {
		m["method"] = step.Method
	}
	if step.Body != "" {
		m["body"] = step.Body
	}
	if step.Headers != nil {
		m["headers"] = step.Headers
	}
	if step.Source != "" {
		m["source"] = step.Source
	}
	if step.Dest != "" {
		m["dest"] = step.Dest
	}
	if step.Content != "" {
		m["content"] = step.Content
	}

	return m
}

// GetSuiteConfig returns the loaded suite configuration
func (r *TestRunner) GetSuiteConfig() *config.SuiteConfig {
	return r.suiteConfig
}

// ListTests returns all tests in the suite
func ListTests(suitePath string) ([]string, error) {
	suitesDir := filepath.Join(suitePath, "suites")

	if _, err := os.Stat(suitesDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	var tests []string

	ucEntries, err := os.ReadDir(suitesDir)
	if err != nil {
		return nil, err
	}

	for _, ucEntry := range ucEntries {
		if !ucEntry.IsDir() || ucEntry.Name()[0] == '.' {
			continue
		}

		ucName := ucEntry.Name()
		ucPath := filepath.Join(suitesDir, ucName)

		tcEntries, err := os.ReadDir(ucPath)
		if err != nil {
			continue
		}

		for _, tcEntry := range tcEntries {
			if !tcEntry.IsDir() || tcEntry.Name()[0] == '.' {
				continue
			}

			tcName := tcEntry.Name()
			testYamlPath := filepath.Join(ucPath, tcName, "test.yaml")

			if _, err := os.Stat(testYamlPath); os.IsNotExist(err) {
				continue
			}

			tests = append(tests, ucName+"/"+tcName)
		}
	}

	return tests, nil
}
