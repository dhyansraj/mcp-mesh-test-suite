package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSuiteConfig(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("writing config.yaml: %v", err)
	}
	return dir
}

// TestPackageVersionsDecodeFromAnyScalar covers the version fields cmd/tsuite
// reports with a run. They are routinely written unquoted, and YAML resolves
// some of those spellings to a non-string type, so a plain `string` field
// would fail the whole config load. Every form must load and keep its text.
func TestPackageVersionsDecodeFromAnyScalar(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		// `0.8.0` has two dots, so YAML already resolves it to !!str.
		{"bare semver", "cli_version: 0.8.0", "0.8.0"},
		{"quoted semver", `cli_version: "0.8.0-beta.9"`, "0.8.0-beta.9"},
		// These are the ones a plain string field would reject.
		{"bare float", "cli_version: 0.8", "0.8"},
		{"bare int", "cli_version: 1", "1"},
		{"trailing zero float", "cli_version: 1.10", "1.10"},
		{"empty value", "cli_version:", ""},
		{"absent", "mode: published", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeSuiteConfig(t, "packages:\n  "+tc.yaml+"\n")

			cfg, err := LoadSuiteConfig(dir)
			if err != nil {
				t.Fatalf("LoadSuiteConfig() error = %v", err)
			}
			if got := cfg.Packages.CLIVersion.String(); got != tc.want {
				t.Errorf("packages.cli_version = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPackageSettingsFullSection(t *testing.T) {
	dir := writeSuiteConfig(t, `suite:
  name: demo
packages:
  mode: local
  local:
    wheels_dir: dist
    packages_dir: pkgs
  cli_version: 0.8.0-beta.9
  sdk_python_version: 0.8.0b9
  sdk_typescript_version: 0.8.0-beta.9
`)

	cfg, err := LoadSuiteConfig(dir)
	if err != nil {
		t.Fatalf("LoadSuiteConfig() error = %v", err)
	}

	for _, tc := range []struct {
		field, got, want string
	}{
		{"mode", cfg.Packages.Mode, "local"},
		{"local.wheels_dir", cfg.Packages.Local.WheelsDir, "dist"},
		{"local.packages_dir", cfg.Packages.Local.PackagesDir, "pkgs"},
		{"cli_version", cfg.Packages.CLIVersion.String(), "0.8.0-beta.9"},
		{"sdk_python_version", cfg.Packages.SDKPythonVersion.String(), "0.8.0b9"},
		{"sdk_typescript_version", cfg.Packages.SDKTypescriptVersion.String(), "0.8.0-beta.9"},
	} {
		if tc.got != tc.want {
			t.Errorf("packages.%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}

	// The typed fields must agree with the raw map interpolation reads, so
	// ${config.packages.cli_version} and the reported run version cannot drift.
	raw, ok := cfg.Raw["packages"].(map[string]any)
	if !ok {
		t.Fatalf("Raw[packages] = %#v, want a map", cfg.Raw["packages"])
	}
	if raw["cli_version"] != cfg.Packages.CLIVersion.String() {
		t.Errorf("Raw[packages][cli_version] = %#v, want %q", raw["cli_version"], cfg.Packages.CLIVersion)
	}
}

// TestPackageVersionRejectsNonScalar keeps the coercion narrow: it accepts any
// scalar spelling, not a mapping or a list.
func TestPackageVersionRejectsNonScalar(t *testing.T) {
	dir := writeSuiteConfig(t, "packages:\n  cli_version:\n    - 0.8.0\n")

	if _, err := LoadSuiteConfig(dir); err == nil {
		t.Fatal("LoadSuiteConfig() error = nil, want an error for a list-valued cli_version")
	}
}

// TestToMapExposesPackageVersions guards the fallback path used when Raw is
// unset: interpolation must still see the version fields.
func TestToMapExposesPackageVersions(t *testing.T) {
	cfg := &SuiteConfig{}
	cfg.Packages.CLIVersion = "0.8.0"
	cfg.Packages.SDKPythonVersion = "0.8.0b9"
	cfg.Packages.SDKTypescriptVersion = "0.8.0-beta.9"

	pkgs, ok := cfg.ToMap()["packages"].(map[string]any)
	if !ok {
		t.Fatalf("ToMap()[packages] = %#v, want a map", cfg.ToMap()["packages"])
	}
	for key, want := range map[string]string{
		"cli_version":            "0.8.0",
		"sdk_python_version":     "0.8.0b9",
		"sdk_typescript_version": "0.8.0-beta.9",
	} {
		if pkgs[key] != want {
			t.Errorf("ToMap()[packages][%s] = %#v, want %q", key, pkgs[key], want)
		}
	}
}
