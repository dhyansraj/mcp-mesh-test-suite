package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// GradleInstallHandler resolves Gradle dependencies
type GradleInstallHandler struct{}

func (h *GradleInstallHandler) Name() string {
	return "gradle-install"
}

func (h *GradleInstallHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	// Get path parameter
	path, hasPath := step["path"].(string)
	if !hasPath {
		return StepResult{
			Success: false,
			Error:   "gradle-install handler requires 'path' field",
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

	// Check if build.gradle or build.gradle.kts exists
	buildGradle := filepath.Join(path, "build.gradle")
	buildGradleKts := filepath.Join(path, "build.gradle.kts")

	_, groovyExists := os.Stat(buildGradle)
	_, ktsExists := os.Stat(buildGradleKts)

	if os.IsNotExist(groovyExists) && os.IsNotExist(ktsExists) {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("neither build.gradle nor build.gradle.kts found in %s", path),
		}
	}

	// Remove file:// repositories by default (set strip_file_repos: false to disable)
	stripFileRepos := true
	if v, ok := step["strip_file_repos"].(bool); ok {
		stripFileRepos = v
	}

	if stripFileRepos {
		// Process both files if they exist
		if groovyExists == nil {
			if err := removeGradleFileRepositories(buildGradle); err != nil {
				return StepResult{
					Success:  false,
					ExitCode: 1,
					Error:    fmt.Sprintf("failed to process build.gradle: %v", err),
				}
			}
		}
		if ktsExists == nil {
			if err := removeGradleFileRepositories(buildGradleKts); err != nil {
				return StepResult{
					Success:  false,
					ExitCode: 1,
					Error:    fmt.Sprintf("failed to process build.gradle.kts: %v", err),
				}
			}
		}
	}

	timeout := 300 * time.Second
	if t, ok := step["timeout"].(int); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	// Determine gradle executable - prefer wrapper if available
	gradleCmd := findGradleExecutable(path)

	// Run gradle dependencies --quiet
	cmd := exec.CommandContext(cmdCtx, gradleCmd, "dependencies", "--quiet")
	cmd.Dir = path
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
				Error:    "gradle dependencies timed out",
			}
		}
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			Error:    fmt.Sprintf("gradle dependencies failed: %v", err),
		}
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

// findGradleExecutable returns the path to the gradle executable.
// Prefers gradlew wrapper if present, otherwise uses system gradle.
func findGradleExecutable(projectPath string) string {
	var wrapperName string
	if runtime.GOOS == "windows" {
		wrapperName = "gradlew.bat"
	} else {
		wrapperName = "gradlew"
	}

	wrapperPath := filepath.Join(projectPath, wrapperName)
	if _, err := os.Stat(wrapperPath); err == nil {
		return wrapperPath
	}

	return "gradle"
}

// removeGradleFileRepositories removes maven repository declarations with file:// URLs
// from Gradle build files. This handles both Groovy DSL and Kotlin DSL syntax.
//
// Groovy DSL patterns:
//   maven { url 'file:///some/local/path' }
//   maven { url "file:///some/local/path" }
//
// Kotlin DSL patterns:
//   maven { url = uri("file:///some/local/path") }
//   maven(url = "file:///some/local/path")
func removeGradleFileRepositories(buildFile string) error {
	data, err := os.ReadFile(buildFile)
	if err != nil {
		return fmt.Errorf("failed to read build file: %w", err)
	}

	content := string(data)

	// Pattern for Groovy DSL: maven { url 'file://...' } or maven { url "file://..." }
	// Also handles variations with whitespace and newlines
	groovyPattern := regexp.MustCompile(`(?s)maven\s*\{\s*url\s*['"]file://[^'"]*['"]\s*\}\s*`)

	// Pattern for Kotlin DSL: maven { url = uri("file://...") }
	kotlinUriPattern := regexp.MustCompile(`(?s)maven\s*\{\s*url\s*=\s*uri\s*\(\s*["']file://[^"']*["']\s*\)\s*\}\s*`)

	// Pattern for Kotlin DSL: maven(url = "file://...")
	kotlinFuncPattern := regexp.MustCompile(`(?s)maven\s*\(\s*url\s*=\s*["']file://[^"']*["']\s*\)\s*`)

	modified := groovyPattern.ReplaceAllString(content, "")
	modified = kotlinUriPattern.ReplaceAllString(modified, "")
	modified = kotlinFuncPattern.ReplaceAllString(modified, "")

	// Only write if modified
	if modified != content {
		if err := os.WriteFile(buildFile, []byte(modified), 0644); err != nil {
			return fmt.Errorf("failed to write build file: %w", err)
		}
	}

	return nil
}
