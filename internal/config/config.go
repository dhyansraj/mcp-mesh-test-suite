package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SuiteConfig represents the top-level config.yaml structure
type SuiteConfig struct {
	Suite      SuiteSettings      `yaml:"suite"`
	Packages   PackageSettings    `yaml:"packages"`
	Docker     DockerSettings     `yaml:"docker"`
	K8s        K8sSettings        `yaml:"k8s"`
	Standalone StandaloneSettings `yaml:"standalone"`
	SSH        SSHSettings        `yaml:"ssh"`
	Execution  ExecutionSettings  `yaml:"execution"`
	Defaults   DefaultSettings    `yaml:"defaults"`
	Reports    ReportSettings     `yaml:"reports"`
	Aliases    map[string]string  `yaml:"aliases"`

	// Raw map for interpolation access
	Raw map[string]any `yaml:"-"`
}

// SuiteSettings contains suite metadata
type SuiteSettings struct {
	Name     string `yaml:"name"`
	Mode     string `yaml:"mode"` // "docker", "standalone", or "k8s"
	Disabled bool   `yaml:"disabled"`
}

// PackageSettings contains package version configuration
type PackageSettings struct {
	Mode  string        `yaml:"mode"` // "local", "published", or "auto"
	Local LocalSettings `yaml:"local"`

	// Versions recorded with each run and reachable from tests as
	// ${config.packages.cli_version}. ScalarString because configs write them
	// unquoted (`cli_version: 0.8` resolves to a YAML float, not a string).
	CLIVersion           ScalarString `yaml:"cli_version"`
	SDKPythonVersion     ScalarString `yaml:"sdk_python_version"`
	SDKTypescriptVersion ScalarString `yaml:"sdk_typescript_version"`
}

// ScalarString is a string that accepts any YAML scalar, keeping the text as
// written. A plain `string` field would fail the whole config load on an
// unquoted version that YAML resolves to another type (`cli_version: 0.8` ->
// !!float, `cli_version: 1` -> !!int); those become "0.8" and "1" here.
type ScalarString string

func (s ScalarString) String() string { return string(s) }

func (s *ScalarString) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: expected a scalar value, got %s", node.Line, node.ShortTag())
	}
	if node.Tag == "!!null" {
		*s = ""
		return nil
	}
	*s = ScalarString(node.Value)
	return nil
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

// K8sSettings contains Kubernetes configuration
type K8sSettings struct {
	Namespace   string `yaml:"namespace"`    // default: "tsuite"
	NFSServer   string `yaml:"nfs_server"`   // e.g., "10.0.0.50"
	NFSPath     string `yaml:"nfs_path"`     // e.g., "/path/to/tests"
	NFSRoot     string `yaml:"nfs_root"`     // NFS export root for symlink resolution (e.g., "/home/dhyanraj/workspace")
	Image       string `yaml:"image"`        // override docker.base_image
	APIUrl      string `yaml:"api_url"`      // e.g., "http://10.0.0.50:9999"
	Kubeconfig  string `yaml:"kubeconfig"`   // optional, defaults to ~/.kube/config
	MemoryLimit string `yaml:"memory_limit"` // pod memory limit, default "4Gi"
	CPULimit    string `yaml:"cpu_limit"`    // pod CPU limit, default "2"
}

// StandaloneSettings contains standalone mode configuration
type StandaloneSettings struct {
	Type string `yaml:"type"` // "local" (default) or "remote"
}

// SSHSettings contains SSH configuration for remote standalone execution
type SSHSettings struct {
	Host      string `yaml:"host"`       // e.g., "beelink1" or "10.0.0.101"
	RunnerDir string `yaml:"runner_dir"` // where to stage runner binary (default: /tmp/tsuite)
	APIUrl    string `yaml:"api_url"`    // API URL reachable from remote host (auto-detect if empty)
	LocalPath string `yaml:"local_path"` // local NFS export path (e.g., "/Users/dhyanraj/workspace")
	MountPath string `yaml:"mount_path"` // remote NFS mount path (e.g., "/mnt/workspace")
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
	Name        string      `yaml:"name"`
	Disabled    bool        `yaml:"disabled"`
	Description string      `yaml:"description"`
	Tags        []string    `yaml:"tags"`
	DependsOn   []string    `yaml:"depends_on"`
	Timeout     int         `yaml:"timeout"`
	PreRun      []Step      `yaml:"pre_run"`
	Test        []Step      `yaml:"test"`
	PostRun     []Step      `yaml:"post_run"`
	Assertions  []Assertion `yaml:"assertions"`

	// Raw map for interpolation access
	Raw map[string]any `yaml:"-"`
}

// Step represents a test step
type Step struct {
	Name    string `yaml:"name"`
	Handler string `yaml:"handler"`
	Command string `yaml:"command,omitempty"`
	Workdir string `yaml:"workdir,omitempty"`
	Capture string `yaml:"capture,omitempty"`
	// Timeout is seconds when given as a number, or a duration string ("5m").
	Timeout      any  `yaml:"timeout,omitempty"`
	IgnoreErrors bool `yaml:"ignore_errors,omitempty"`

	// Handler-specific fields
	Path    string            `yaml:"path,omitempty"`    // npm-install, pip-install
	Seconds int               `yaml:"seconds,omitempty"` // wait
	URL     string            `yaml:"url,omitempty"`     // http
	Method  string            `yaml:"method,omitempty"`  // http
	Body    any               `yaml:"body,omitempty"`    // http: string sent verbatim, map/list sent as JSON
	Headers map[string]string `yaml:"headers,omitempty"` // http
	Source  string            `yaml:"source,omitempty"`  // file, secrets
	Dest    string            `yaml:"dest,omitempty"`    // file
	Content string            `yaml:"content,omitempty"` // file
	Target  string            `yaml:"target,omitempty"`  // secrets
	Keys    []string          `yaml:"keys,omitempty"`    // secrets

	// Probe fields
	Interval         any    `yaml:"interval,omitempty"`          // probe: seconds or duration string
	Until            string `yaml:"until,omitempty"`             // probe: assertion expression
	SuccessThreshold int    `yaml:"success_threshold,omitempty"` // probe: consecutive passes
	OnFailure        string `yaml:"on_failure,omitempty"`        // probe: diagnostic command

	// Routine fields
	Routine string         `yaml:"routine,omitempty"`
	Params  map[string]any `yaml:"params,omitempty"`

	// Extra collects every key without a dedicated field above (operation,
	// packages, type, strip_file_repos, ...) so handler options stay reachable
	// without a struct field per handler. Values keep their YAML types.
	Extra map[string]any `yaml:",inline"`
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
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Steps       []Step `yaml:"steps"`
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

// UseCaseConfig represents a uc.yaml file
type UseCaseConfig struct {
	Disabled bool `yaml:"disabled"`
}

// LoadUseCaseConfig loads uc.yaml from a use case directory
func LoadUseCaseConfig(ucPath string) (*UseCaseConfig, error) {
	ucYamlPath := filepath.Join(ucPath, "uc.yaml")

	data, err := os.ReadFile(ucYamlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &UseCaseConfig{}, nil
		}
		return nil, fmt.Errorf("reading uc.yaml: %w", err)
	}

	var config UseCaseConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing uc.yaml: %w", err)
	}

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
		"cli_version":            c.Packages.CLIVersion.String(),
		"sdk_python_version":     c.Packages.SDKPythonVersion.String(),
		"sdk_typescript_version": c.Packages.SDKTypescriptVersion.String(),
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
	m["standalone"] = map[string]any{
		"type": c.Standalone.Type,
	}
	m["ssh"] = map[string]any{
		"host":       c.SSH.Host,
		"runner_dir": c.SSH.RunnerDir,
		"api_url":    c.SSH.APIUrl,
		"local_path": c.SSH.LocalPath,
		"mount_path": c.SSH.MountPath,
	}

	return m
}

// ResolveWithSecrets resolves paths using workspace root secrets.
// REMOTE_WORKSPACE_ROOT: used for k8s NFS paths and SSH mount_path
// LOCAL_WORKSPACE_ROOT: used for SSH local_path and deriving k8s nfs_path
func (c *SuiteConfig) ResolveWithSecrets(secrets map[string]string, suitePath string) {
	remoteRoot := strings.TrimRight(secrets["REMOTE_WORKSPACE_ROOT"], "/")
	localRoot := strings.TrimRight(secrets["LOCAL_WORKSPACE_ROOT"], "/")

	// K8s: resolve relative NFS paths
	if remoteRoot != "" {
		if c.K8s.NFSPath != "" && !filepath.IsAbs(c.K8s.NFSPath) {
			c.K8s.NFSPath = remoteRoot + "/" + c.K8s.NFSPath
		}
		if c.K8s.NFSRoot == "" {
			c.K8s.NFSRoot = remoteRoot
		} else if !filepath.IsAbs(c.K8s.NFSRoot) {
			c.K8s.NFSRoot = remoteRoot + "/" + c.K8s.NFSRoot
		}
	}

	// K8s: derive nfs_path from suite path when not configured
	if c.K8s.NFSPath == "" && localRoot != "" && remoteRoot != "" && strings.HasPrefix(suitePath, localRoot) {
		relPath := strings.TrimPrefix(suitePath, localRoot)
		relPath = strings.TrimPrefix(relPath, "/")
		c.K8s.NFSPath = remoteRoot + "/" + relPath
	}

	// SSH: default local_path and mount_path from secrets
	if c.SSH.LocalPath == "" && localRoot != "" {
		c.SSH.LocalPath = localRoot
	}
	if c.SSH.MountPath == "" && remoteRoot != "" {
		c.SSH.MountPath = remoteRoot
	}
}
