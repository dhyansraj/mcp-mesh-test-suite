package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SuiteConfig represents the top-level config.yaml structure
type SuiteConfig struct {
	Suite      SuiteSettings      `yaml:"suite"`
	Packages   PackageSettings    `yaml:"packages"`
	Docker     DockerSettings     `yaml:"docker"`
	Execution  ExecutionSettings  `yaml:"execution"`
	Defaults   DefaultSettings    `yaml:"defaults"`
	Reports    ReportSettings     `yaml:"reports"`
	Aliases    map[string]string  `yaml:"aliases"`

	// Raw map for interpolation access
	Raw map[string]any `yaml:"-"`
}

// SuiteSettings contains suite metadata
type SuiteSettings struct {
	Name string `yaml:"name"`
	Mode string `yaml:"mode"` // "docker" or "standalone"
}

// PackageSettings contains package version configuration
type PackageSettings struct {
	Mode  string         `yaml:"mode"`  // "local", "published", or "auto"
	Local LocalSettings  `yaml:"local"`
}

// LocalSettings contains paths for local package mode
type LocalSettings struct {
	WheelsDir   string `yaml:"wheels_dir"`
	PackagesDir string `yaml:"packages_dir"`
}

// DockerSettings contains Docker configuration
type DockerSettings struct {
	BaseImage string `yaml:"base_image"`
	Network   string `yaml:"network"`
}

// ExecutionSettings contains test execution configuration
type ExecutionSettings struct {
	MaxWorkers int `yaml:"max_workers"`
	Timeout    int `yaml:"timeout"` // seconds
}

// DefaultSettings contains default values for tests
type DefaultSettings struct {
	Timeout  int `yaml:"timeout"`
	Parallel int `yaml:"parallel"` // deprecated
	Retry    int `yaml:"retry"`
}

// ReportSettings contains report configuration
type ReportSettings struct {
	OutputDir string   `yaml:"output_dir"`
	Formats   []string `yaml:"formats"`
	KeepLast  int      `yaml:"keep_last"`
}

// TestConfig represents a test.yaml file
type TestConfig struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Tags        []string            `yaml:"tags"`
	Timeout     int                 `yaml:"timeout"`
	PreRun      []Step              `yaml:"pre_run"`
	Test        []Step              `yaml:"test"`
	PostRun     []Step              `yaml:"post_run"`
	Assertions  []Assertion         `yaml:"assertions"`

	// Raw map for interpolation access
	Raw map[string]any `yaml:"-"`
}

// Step represents a test step
type Step struct {
	Name         string         `yaml:"name"`
	Handler      string         `yaml:"handler"`
	Command      string         `yaml:"command,omitempty"`
	Workdir      string         `yaml:"workdir,omitempty"`
	Capture      string         `yaml:"capture,omitempty"`
	Timeout      int            `yaml:"timeout,omitempty"`
	IgnoreErrors bool           `yaml:"ignore_errors,omitempty"`

	// Handler-specific fields
	Path       string         `yaml:"path,omitempty"`        // npm-install, pip-install
	Seconds    int            `yaml:"seconds,omitempty"`     // wait
	URL        string         `yaml:"url,omitempty"`         // http
	Method     string         `yaml:"method,omitempty"`      // http
	Body       string         `yaml:"body,omitempty"`        // http
	Headers    map[string]string `yaml:"headers,omitempty"`  // http
	Source     string         `yaml:"source,omitempty"`      // file
	Dest       string         `yaml:"dest,omitempty"`        // file
	Content    string         `yaml:"content,omitempty"`     // file

	// Routine fields
	Routine string         `yaml:"routine,omitempty"`
	Params  map[string]any `yaml:"params,omitempty"`

	// Raw map for interpolation
	Raw map[string]any `yaml:"-"`
}

// Assertion represents a test assertion
type Assertion struct {
	Expr    string `yaml:"expr"`
	Message string `yaml:"message"`
}

// GlobalRoutinesConfig represents global/routines.yaml
type GlobalRoutinesConfig struct {
	Routines map[string]RoutineDefinition `yaml:"routines"`
}

// UseCaseRoutinesConfig represents uc_*/routines.yaml
type UseCaseRoutinesConfig struct {
	Routines map[string]RoutineDefinition `yaml:"routines"`
}

// RoutineDefinition represents a reusable routine
type RoutineDefinition struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description,omitempty"`
	Steps       []Step         `yaml:"steps"`
}

// LoadSuiteConfig loads config.yaml from a suite path
func LoadSuiteConfig(suitePath string) (*SuiteConfig, error) {
	configPath := filepath.Join(suitePath, "config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config.yaml: %w", err)
	}

	var config SuiteConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config.yaml: %w", err)
	}

	// Also keep raw map for interpolation
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config.yaml as map: %w", err)
	}
	config.Raw = raw

	return &config, nil
}

// LoadTestConfig loads test.yaml from a test case path
func LoadTestConfig(testPath string) (*TestConfig, error) {
	testYamlPath := filepath.Join(testPath, "test.yaml")

	data, err := os.ReadFile(testYamlPath)
	if err != nil {
		return nil, fmt.Errorf("reading test.yaml: %w", err)
	}

	var config TestConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing test.yaml: %w", err)
	}

	// Also keep raw map for interpolation
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing test.yaml as map: %w", err)
	}
	config.Raw = raw

	return &config, nil
}

// LoadGlobalRoutines loads global/routines.yaml
func LoadGlobalRoutines(suitePath string) (*GlobalRoutinesConfig, error) {
	routinesPath := filepath.Join(suitePath, "global", "routines.yaml")

	data, err := os.ReadFile(routinesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalRoutinesConfig{Routines: make(map[string]RoutineDefinition)}, nil
		}
		return nil, fmt.Errorf("reading global routines.yaml: %w", err)
	}

	var config GlobalRoutinesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing global routines.yaml: %w", err)
	}

	if config.Routines == nil {
		config.Routines = make(map[string]RoutineDefinition)
	}

	return &config, nil
}

// LoadUseCaseRoutines loads uc_*/routines.yaml
func LoadUseCaseRoutines(useCasePath string) (*UseCaseRoutinesConfig, error) {
	routinesPath := filepath.Join(useCasePath, "routines.yaml")

	data, err := os.ReadFile(routinesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &UseCaseRoutinesConfig{Routines: make(map[string]RoutineDefinition)}, nil
		}
		return nil, fmt.Errorf("reading use case routines.yaml: %w", err)
	}

	var config UseCaseRoutinesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing use case routines.yaml: %w", err)
	}

	if config.Routines == nil {
		config.Routines = make(map[string]RoutineDefinition)
	}

	return &config, nil
}

// ToMap converts SuiteConfig to a map for interpolation
func (c *SuiteConfig) ToMap() map[string]any {
	if c.Raw != nil {
		return c.Raw
	}

	// Build map from structured fields
	m := make(map[string]any)
	m["suite"] = map[string]any{
		"name": c.Suite.Name,
		"mode": c.Suite.Mode,
	}
	m["packages"] = map[string]any{
		"mode": c.Packages.Mode,
		"local": map[string]any{
			"wheels_dir":   c.Packages.Local.WheelsDir,
			"packages_dir": c.Packages.Local.PackagesDir,
		},
	}
	m["docker"] = map[string]any{
		"base_image": c.Docker.BaseImage,
		"network":    c.Docker.Network,
	}
	m["execution"] = map[string]any{
		"max_workers": c.Execution.MaxWorkers,
		"timeout":     c.Execution.Timeout,
	}
	m["defaults"] = map[string]any{
		"timeout":  c.Defaults.Timeout,
		"parallel": c.Defaults.Parallel,
		"retry":    c.Defaults.Retry,
	}
	m["reports"] = map[string]any{
		"output_dir": c.Reports.OutputDir,
		"formats":    c.Reports.Formats,
		"keep_last":  c.Reports.KeepLast,
	}
	m["aliases"] = c.Aliases

	return m
}
