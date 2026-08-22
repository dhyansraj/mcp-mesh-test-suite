package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"

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

	// Resolve the timeout before touching the project, so a malformed step is
	// rejected rather than rewriting pom.xml on its way to failing.
	timeout, err := durationField(step, "timeout", defaultInstallTimeout)
	if err != nil {
		return StepResult{
			Success: false,
			Error:   fmt.Sprintf("maven-install handler: %v", err),
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

	// Align mcp-mesh SDK version from container's local m2 repository
	alignVersion := true
	if v, ok := step["align_version"].(bool); ok {
		alignVersion = v
	}

	var versionOutput string
	if alignVersion {
		m2Repo := "/root/.m2/repository"
		if v, ok := step["m2_repo"].(string); ok && v != "" {
			m2Repo = v
		}

		detectedVersion, err := alignMeshVersion(pomFile, m2Repo)
		if err != nil {
			fmt.Printf("Warning: failed to align mcp-mesh version: %v\n", err)
		} else if detectedVersion != "" {
			versionOutput = fmt.Sprintf("Aligned mcp-mesh version to %s\n", detectedVersion)
			fmt.Print(versionOutput)
		}
	}

	cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	// Run mvn dependency:resolve
	cmd := exec.CommandContext(cmdCtx, "mvn", "dependency:resolve", "-q")
	cmd.Dir = path
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return StepResult{
				Success:  false,
				ExitCode: 124,
				Stdout:   versionOutput + stdout.String(),
				Stderr:   stderr.String(),
				Error:    "mvn dependency:resolve timed out",
			}
		}
		return StepResult{
			Success:  false,
			ExitCode: 1,
			Stdout:   versionOutput + stdout.String(),
			Stderr:   stderr.String(),
			Error:    fmt.Sprintf("mvn dependency:resolve failed: %v", err),
		}
	}

	return StepResult{
		Success:  true,
		ExitCode: 0,
		Stdout:   versionOutput + stdout.String(),
		Stderr:   stderr.String(),
	}
}

// alignMeshVersion detects the mcp-mesh SDK version from the container's local m2 repository
// and updates the <mcp-mesh.version> property in pom.xml to match.
// Returns the detected version, or empty string if no version was found.
func alignMeshVersion(pomFile string, m2Repo string) (string, error) {
	sdkPath := filepath.Join(m2Repo, "io", "mcp-mesh", "mcp-mesh-spring-boot-starter")

	entries, err := os.ReadDir(sdkPath)
	if err != nil {
		if os.IsNotExist(err) {
			// m2 path doesn't exist - skip silently
			return "", nil
		}
		return "", fmt.Errorf("failed to read SDK directory %s: %w", sdkPath, err)
	}

	// Collect version directories
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}

	if len(versions) == 0 {
		return "", nil
	}

	// Sort and pick the newest (last after sort)
	sort.Strings(versions)
	detectedVersion := versions[len(versions)-1]

	// Read pom.xml and update <mcp-mesh.version>
	data, err := os.ReadFile(pomFile)
	if err != nil {
		return "", fmt.Errorf("failed to read pom.xml: %w", err)
	}

	content := string(data)

	versionPattern := regexp.MustCompile(`<mcp-mesh\.version>[^<]*</mcp-mesh\.version>`)
	if !versionPattern.MatchString(content) {
		// No <mcp-mesh.version> property found - nothing to align
		return "", nil
	}

	replacement := fmt.Sprintf("<mcp-mesh.version>%s</mcp-mesh.version>", detectedVersion)
	modified := versionPattern.ReplaceAllString(content, replacement)

	if modified != content {
		if err := os.WriteFile(pomFile, []byte(modified), 0644); err != nil {
			return "", fmt.Errorf("failed to write pom.xml: %w", err)
		}
	}

	return detectedVersion, nil
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
