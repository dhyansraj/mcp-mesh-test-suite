package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// MavenInstallHandler resolves Maven dependencies
type MavenInstallHandler struct{}

func (h *MavenInstallHandler) Name() string {
	return "maven-install"
}

func (h *MavenInstallHandler) Execute(step map[string]any, ctx *interpolate.Context) StepResult {
	// Get path parameter
	path, hasPath := step["path"].(string)
	if !hasPath {
		return StepResult{
			Success: false,
			Error:   "maven-install handler requires 'path' field",
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

	// Check if pom.xml exists
	pomFile := filepath.Join(path, "pom.xml")
	if _, err := os.Stat(pomFile); os.IsNotExist(err) {
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Error:    fmt.Sprintf("pom.xml not found in %s", path),
		}
	}

	// Remove file:// repositories by default (set strip_file_repos: false to disable)
	stripFileRepos := true
	if v, ok := step["strip_file_repos"].(bool); ok {
		stripFileRepos = v
	}

	if stripFileRepos {
		if err := removeFileRepositories(pomFile); err != nil {
			return StepResult{
				Success:  false,
				ExitCode: 1,
				Error:    fmt.Sprintf("failed to process pom.xml: %v", err),
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

	// Run mvn dependency:resolve
	cmd := exec.CommandContext(cmdCtx, "mvn", "dependency:resolve", "-q")
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
				Error:    "mvn dependency:resolve timed out",
			}
		}
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			Error:    fmt.Sprintf("mvn dependency:resolve failed: %v", err),
		}
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

// removeFileRepositories removes <repository> blocks with file:// URLs from pom.xml
// This is needed because file:// paths that work on the host don't exist in containers.
// The SDK JARs are pre-installed in /root/.m2/repository in the container image.
func removeFileRepositories(pomFile string) error {
	data, err := os.ReadFile(pomFile)
	if err != nil {
		return fmt.Errorf("failed to read pom.xml: %w", err)
	}

	content := string(data)

	// Match <repository>...</repository> blocks containing file:// URLs
	// Using (?s) for DOTALL mode so . matches newlines
	repoPattern := regexp.MustCompile(`(?s)<repository>\s*<id>[^<]*</id>\s*<url>file://[^<]*</url>\s*</repository>\s*`)

	modified := repoPattern.ReplaceAllString(content, "")

	// Only write if modified
	if modified != content {
		if err := os.WriteFile(pomFile, []byte(modified), 0644); err != nil {
			return fmt.Errorf("failed to write pom.xml: %w", err)
		}
	}

	return nil
}
