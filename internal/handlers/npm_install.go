package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// NpmInstallHandler installs npm packages
type NpmInstallHandler struct{}

func (h *NpmInstallHandler) Name() string {
	return "npm-install"
}

func (h *NpmInstallHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	// Get path
	path, _ := step["path"].(string)
	if path == "" {
		return StepResult{
			Success: false,
			Error:   "npm-install handler requires 'path' field",
		}
	}

	// Interpolate path
	path, _ = interpolate.Interpolate(path, ctx)

	// Make path absolute if not already
	if !filepath.IsAbs(path) {
		workdir := ctx.Workdir
		if workdir == "" {
			workdir = "/workspace"
		}
		path = filepath.Join(workdir, path)
	}

	// Check if package.json exists
	packageJSON := filepath.Join(path, "package.json")
	if _, err := os.Stat(packageJSON); os.IsNotExist(err) {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("package.json not found at %s", path),
		}
	}

	// Replace file: dependencies by default (set replace_file_deps: false to disable)
	replaceFileDeps := true
	if v, ok := step["replace_file_deps"].(bool); ok {
		replaceFileDeps = v
	}
	// Legacy support for strip_file_deps
	if v, ok := step["strip_file_deps"].(bool); ok {
		replaceFileDeps = v
	}

	if replaceFileDeps {
		// Get version from config if available, otherwise use "*"
		version := "*"
		if packages, ok := ctx.Config["packages"].(map[string]any); ok {
			if v, ok := packages["sdk_typescript_version"].(string); ok && v != "" {
				version = v
			}
		}

		modified, err := replaceFileDepependencies(packageJSON, version)
		if err != nil {
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Error:    fmt.Sprintf("failed to replace file: dependencies: %v", err),
			}
		}

		// Delete package-lock.json if file: deps were replaced
		// The lock file contains stale file: references that would cause npm install to fail
		if modified {
			lockFile := filepath.Join(path, "package-lock.json")
			os.Remove(lockFile)
		}
	}

	// Determine package mode (local or published)
	// Check for /packages directory (local mode) or default to published
	mode := "published"
	if _, err := os.Stat("/packages"); err == nil {
		mode = "local"
	}

	timeout := 300 * time.Second
	if t, ok := step["timeout"].(int); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	if mode == "local" {
		// Local mode: rewrite package.json deps to point at local tarballs,
		// then run a single npm install so transitive deps also resolve locally.
		rewritten, err := rewriteDepsToLocalTarballs(packageJSON)
		if err != nil {
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Error:    fmt.Sprintf("failed to rewrite deps to local tarballs: %v", err),
			}
		}
		if rewritten {
			// Remove stale lock file so npm re-resolves everything
			os.Remove(filepath.Join(path, "package-lock.json"))
		}

		cmd := exec.CommandContext(cmdCtx, "bash", "-c", `
			cd "$1"
			rm -rf node_modules 2>/dev/null || true
			npm install --legacy-peer-deps
		`, "bash", path)

		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		if err != nil {
			if cmdCtx.Err() == context.DeadlineExceeded {
				return StepResult{
					Success:  false,
					ExitCode: 124,
					Stdout:   stdout.String(),
					Stderr:   stderr.String(),
					Error:    "npm install timed out",
				}
			}
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Error:    fmt.Sprintf("npm install failed: %v", err),
			}
		}
	} else {
		// Published mode: just run npm install
		// First remove node_modules to avoid platform mismatch
		cmd := exec.CommandContext(cmdCtx, "bash", "-c", `
			cd "$1"
			rm -rf node_modules 2>/dev/null || true
			npm install --legacy-peer-deps
		`, "bash", path)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err != nil {
			if cmdCtx.Err() == context.DeadlineExceeded {
				return StepResult{
					Success:  false,
					ExitCode: 124,
					Stdout:   stdout.String(),
					Stderr:   stderr.String(),
					Error:    "npm install timed out",
				}
			}
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Error:    fmt.Sprintf("npm install failed: %v", err),
			}
		}
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

// replaceFileDepependencies replaces file: dependencies in package.json with a version
// This is useful when examples reference local packages via file: paths
// that don't exist in the container. The version is replaced so npm install
// can resolve the package, and local .tgz packages can override afterward.
func replaceFileDepependencies(packageJSONPath string, version string) (bool, error) {
	// Read package.json
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return false, fmt.Errorf("failed to read package.json: %w", err)
	}

	// Parse as generic map to preserve structure
	var pkg map[string]any
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false, fmt.Errorf("failed to parse package.json: %w", err)
	}

	modified := false

	// Replace file: deps in dependencies
	if deps, ok := pkg["dependencies"].(map[string]any); ok {
		for name, ver := range deps {
			if v, ok := ver.(string); ok && strings.HasPrefix(v, "file:") {
				deps[name] = version
				modified = true
			}
		}
	}

	// Replace file: deps in devDependencies
	if deps, ok := pkg["devDependencies"].(map[string]any); ok {
		for name, ver := range deps {
			if v, ok := ver.(string); ok && strings.HasPrefix(v, "file:") {
				deps[name] = version
				modified = true
			}
		}
	}

	// Replace file: deps in optionalDependencies
	if deps, ok := pkg["optionalDependencies"].(map[string]any); ok {
		for name, ver := range deps {
			if v, ok := ver.(string); ok && strings.HasPrefix(v, "file:") {
				deps[name] = version
				modified = true
			}
		}
	}

	// Replace file: deps in peerDependencies
	if deps, ok := pkg["peerDependencies"].(map[string]any); ok {
		for name, ver := range deps {
			if v, ok := ver.(string); ok && strings.HasPrefix(v, "file:") {
				deps[name] = version
				modified = true
			}
		}
	}

	// Only write if modified
	if modified {
		// Marshal with indentation to preserve readability
		newData, err := json.MarshalIndent(pkg, "", "  ")
		if err != nil {
			return false, fmt.Errorf("failed to marshal package.json: %w", err)
		}

		// Write back
		if err := os.WriteFile(packageJSONPath, newData, 0644); err != nil {
			return false, fmt.Errorf("failed to write package.json: %w", err)
		}
	}

	return modified, nil
}

// getPackageNameFromTarball extracts the package name from a .tgz tarball
// by reading the embedded package/package.json.
func getPackageNameFromTarball(tgzPath string) (string, error) {
	f, err := os.Open(tgzPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("package.json not found in tarball %s", filepath.Base(tgzPath))
		}
		if err != nil {
			return "", fmt.Errorf("reading tarball %s: %w", filepath.Base(tgzPath), err)
		}
		if hdr.Name == "package/package.json" {
			var pkg struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(tr).Decode(&pkg); err != nil {
				return "", fmt.Errorf("parsing package.json in %s: %w", filepath.Base(tgzPath), err)
			}
			if pkg.Name == "" {
				return "", fmt.Errorf("empty name in package.json of %s", filepath.Base(tgzPath))
			}
			return pkg.Name, nil
		}
	}
}

// rewriteDepsToLocalTarballs scans /packages/*.tgz, extracts each package name,
// and rewrites matching dependencies in the given package.json to use file: paths.
// This ensures that a single npm install resolves both direct and transitive deps
// from local tarballs instead of the npm registry.
func rewriteDepsToLocalTarballs(packageJSONPath string) (bool, error) {
	tarballs, err := filepath.Glob("/packages/*.tgz")
	if err != nil || len(tarballs) == 0 {
		return false, nil
	}

	// Build map: package name -> tgz path
	localPkgs := make(map[string]string)
	for _, tgz := range tarballs {
		name, err := getPackageNameFromTarball(tgz)
		if err != nil {
			// Skip tarballs we can't parse
			continue
		}
		localPkgs[name] = tgz
	}
	if len(localPkgs) == 0 {
		return false, nil
	}

	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return false, fmt.Errorf("failed to read package.json: %w", err)
	}

	var pkg map[string]any
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false, fmt.Errorf("failed to parse package.json: %w", err)
	}

	modified := false
	depSections := []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"}
	for _, section := range depSections {
		deps, ok := pkg[section].(map[string]any)
		if !ok {
			continue
		}
		for name := range deps {
			if tgzPath, found := localPkgs[name]; found {
				deps[name] = "file:" + tgzPath
				modified = true
			}
		}
	}

	if !modified {
		return false, nil
	}

	newData, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return false, fmt.Errorf("failed to marshal package.json: %w", err)
	}

	if err := os.WriteFile(packageJSONPath, newData, 0644); err != nil {
		return false, fmt.Errorf("failed to write package.json: %w", err)
	}

	return true, nil
}
