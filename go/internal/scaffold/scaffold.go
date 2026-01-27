// Package scaffold generates test cases from agent directories.
package scaffold

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// AgentInfo holds information about a detected agent.
type AgentInfo struct {
	Path            string
	Name            string
	AgentType       string // "typescript" or "python"
	EntryPoint      string // e.g., "src/index.ts" or "main.py"
	HasRequirements bool
	PackageJSON     map[string]interface{}
}

// Config holds configuration for scaffold operation.
type Config struct {
	SuitePath        string
	UCName           string
	TCName           string
	Agents           []AgentInfo
	ArtifactLevel    string // "tc" or "uc"
	TestName         string
	DryRun           bool
	Force            bool
	SkipArtifactCopy bool
	UseSymlinks      bool   // Create symlinks instead of copying artifacts
	FlatScriptDir    string // For --filter mode: directory containing flat scripts
	Filter           string // Glob pattern for flat script discovery (e.g., "*.py")
}

// ValidateSuite checks that suite exists and has config.yaml.
func ValidateSuite(suitePath string) error {
	if _, err := os.Stat(suitePath); os.IsNotExist(err) {
		return fmt.Errorf("suite path does not exist: %s", suitePath)
	}

	configFile := filepath.Join(suitePath, "config.yaml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return fmt.Errorf("not a valid suite: %s\nMissing config.yaml", suitePath)
	}

	return nil
}

// ValidateAgentDir validates agent directory and detects type.
func ValidateAgentDir(agentPath string) (*AgentInfo, error) {
	info, err := os.Stat(agentPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("agent directory does not exist: %s", agentPath)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", agentPath)
	}

	packageJSON := filepath.Join(agentPath, "package.json")
	mainPy := filepath.Join(agentPath, "main.py")
	requirementsTxt := filepath.Join(agentPath, "requirements.txt")

	if _, err := os.Stat(packageJSON); err == nil {
		// TypeScript agent
		data, err := os.ReadFile(packageJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to read package.json: %w", err)
		}

		var pkgData map[string]interface{}
		if err := json.Unmarshal(data, &pkgData); err != nil {
			return nil, fmt.Errorf("invalid package.json in %s", agentPath)
		}

		// Determine entry point
		entryPoint := "src/index.ts"
		if _, err := os.Stat(filepath.Join(agentPath, "src", "index.ts")); err == nil {
			entryPoint = "src/index.ts"
		} else if _, err := os.Stat(filepath.Join(agentPath, "index.ts")); err == nil {
			entryPoint = "index.ts"
		}

		return &AgentInfo{
			Path:        agentPath,
			Name:        filepath.Base(agentPath),
			AgentType:   "typescript",
			EntryPoint:  entryPoint,
			PackageJSON: pkgData,
		}, nil
	}

	if _, err := os.Stat(mainPy); err == nil {
		// Python agent
		_, hasReqs := os.Stat(requirementsTxt)
		return &AgentInfo{
			Path:            agentPath,
			Name:            filepath.Base(agentPath),
			AgentType:       "python",
			EntryPoint:      "main.py",
			HasRequirements: hasReqs == nil,
		}, nil
	}

	return nil, fmt.Errorf("cannot detect agent type for: %s\nExpected package.json (TypeScript) or main.py (Python)", agentPath)
}

// DiscoverScriptsByFilter scans a directory for files matching the filter pattern.
// Used for flat directories containing standalone scripts (e.g., examples/simple/*.py).
// Returns AgentInfo for each matched file, where the entry point is the file itself.
func DiscoverScriptsByFilter(dirPath string, filter string) ([]AgentInfo, error) {
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("directory does not exist: %s", dirPath)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dirPath)
	}

	// Match files using glob pattern
	pattern := filepath.Join(dirPath, filter)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern '%s': %w", filter, err)
	}

	var agents []AgentInfo
	for _, match := range matches {
		fileInfo, err := os.Stat(match)
		if err != nil || fileInfo.IsDir() {
			continue // Skip directories and unreadable files
		}

		filename := filepath.Base(match)
		name := strings.TrimSuffix(filename, filepath.Ext(filename))

		// Determine agent type from extension
		agentType := ""
		switch filepath.Ext(filename) {
		case ".py":
			agentType = "python"
		case ".ts":
			agentType = "typescript"
		default:
			// Skip files with unrecognized extensions
			continue
		}

		agents = append(agents, AgentInfo{
			Path:       dirPath, // All scripts share the same parent directory
			Name:       name,
			AgentType:  agentType,
			EntryPoint: filename, // Entry point is the file itself
		})
	}

	return agents, nil
}

// ValidateNoParentDirs ensures no agent path is a parent of another.
func ValidateNoParentDirs(agentPaths []string) error {
	for i, path1 := range agentPaths {
		abs1, _ := filepath.Abs(path1)
		for j, path2 := range agentPaths {
			if i != j {
				abs2, _ := filepath.Abs(path2)
				rel, err := filepath.Rel(abs1, abs2)
				if err == nil && !strings.HasPrefix(rel, "..") {
					return fmt.Errorf("parent directory conflict:\n  %s is parent of %s", abs1, abs2)
				}
			}
		}
	}
	return nil
}

// cleanNpmLocalReferences cleans up local file: references in package.json.
func cleanNpmLocalReferences(packageJSONPath string) (bool, error) {
	const mcpmeshDefaultVersion = "0.8.0-beta.9"

	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return false, nil
	}

	var pkgData map[string]interface{}
	if err := json.Unmarshal(data, &pkgData); err != nil {
		return false, nil
	}

	changed := false
	for _, depKey := range []string{"dependencies", "devDependencies"} {
		deps, ok := pkgData[depKey].(map[string]interface{})
		if !ok {
			continue
		}
		for pkgName, version := range deps {
			versionStr, ok := version.(string)
			if !ok {
				continue
			}
			if strings.HasPrefix(versionStr, "file:") {
				if strings.HasPrefix(pkgName, "@mcpmesh/") {
					deps[pkgName] = mcpmeshDefaultVersion
				} else {
					deps[pkgName] = "*"
				}
				changed = true
			}
		}
	}

	if changed {
		// Use Encoder with SetEscapeHTML(false) to prevent escaping >, <, &
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(pkgData); err != nil {
			return false, err
		}
		if err := os.WriteFile(packageJSONPath, buf.Bytes(), 0644); err != nil {
			return false, err
		}
	}

	return changed, nil
}

// copyDir copies a directory recursively with optional ignore function.
func copyDir(src, dst string, ignore func(string) bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if ignore != nil && ignore(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

// copyFile copies a single file.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// copyAgentToArtifacts copies agent directory to artifacts.
func copyAgentToArtifacts(agent *AgentInfo, artifactsDir string, dryRun bool) (string, error) {
	targetPath := filepath.Join(artifactsDir, agent.Name)

	if dryRun {
		fmt.Printf("  Would copy: %s → %s\n", agent.Path, targetPath)
		return targetPath, nil
	}

	// Remove existing if present
	os.RemoveAll(targetPath)
	os.MkdirAll(targetPath, 0755)

	if agent.AgentType == "typescript" {
		return targetPath, copyTypeScriptAgent(agent, targetPath)
	}
	return targetPath, copyPythonAgent(agent, targetPath)
}

// symlinkAgentToArtifacts creates a symlink to agent directory in artifacts.
// This is useful for testing existing examples without copying them.
func symlinkAgentToArtifacts(agent *AgentInfo, artifactsDir string, dryRun bool) (string, error) {
	targetPath := filepath.Join(artifactsDir, agent.Name)

	// Get absolute path of agent directory for the symlink target
	absAgentPath, err := filepath.Abs(agent.Path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	if dryRun {
		fmt.Printf("  Would symlink: %s → %s\n", targetPath, absAgentPath)
		return targetPath, nil
	}

	// Remove existing if present (file, dir, or symlink)
	os.RemoveAll(targetPath)

	// Create symlink
	if err := os.Symlink(absAgentPath, targetPath); err != nil {
		return "", fmt.Errorf("failed to create symlink: %w", err)
	}

	return targetPath, nil
}

// copyTypeScriptAgent copies TypeScript agent using whitelist approach.
func copyTypeScriptAgent(agent *AgentInfo, targetPath string) error {
	source := agent.Path

	// Essential files
	for _, filename := range []string{"package.json", "tsconfig.json"} {
		srcFile := filepath.Join(source, filename)
		if _, err := os.Stat(srcFile); err == nil {
			copyFile(srcFile, filepath.Join(targetPath, filename))
		}
	}

	// Determine source directory from entry point
	entryDir := "src"
	if strings.Contains(agent.EntryPoint, "/") {
		entryDir = strings.Split(agent.EntryPoint, "/")[0]
	}

	srcDir := filepath.Join(source, entryDir)
	if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
		copyDir(srcDir, filepath.Join(targetPath, entryDir), nil)
	}

	// Copy prompts directory if exists
	promptsDir := filepath.Join(source, "prompts")
	if info, err := os.Stat(promptsDir); err == nil && info.IsDir() {
		copyDir(promptsDir, filepath.Join(targetPath, "prompts"), nil)
	}

	// Clean npm local references
	pkgJSON := filepath.Join(targetPath, "package.json")
	if changed, _ := cleanNpmLocalReferences(pkgJSON); changed {
		fmt.Printf("  Cleaned local npm references in %s/package.json\n", agent.Name)
	}

	return nil
}

// copyPythonAgent copies Python agent using blacklist approach.
func copyPythonAgent(agent *AgentInfo, targetPath string) error {
	excludePatterns := map[string]bool{
		"node_modules": true, "__pycache__": true, ".venv": true, "venv": true,
		"env": true, ".env": true, ".git": true, ".gitignore": true,
		".pytest_cache": true, ".mypy_cache": true, ".ruff_cache": true,
		"dist": true, "build": true, ".idea": true, ".vscode": true, ".DS_Store": true,
		"Dockerfile": true, ".dockerignore": true, "docker-compose.yml": true,
		"README.md": true, "LICENSE": true,
	}

	excludeExtensions := map[string]bool{
		".pyc": true, ".pyo": true, ".db": true, ".db-shm": true, ".db-wal": true,
	}

	ignore := func(relPath string) bool {
		base := filepath.Base(relPath)
		if excludePatterns[base] {
			return true
		}
		ext := filepath.Ext(base)
		if excludeExtensions[ext] {
			return true
		}
		if strings.HasSuffix(base, ".egg-info") {
			return true
		}
		return false
	}

	os.RemoveAll(targetPath)
	return copyDir(agent.Path, targetPath, ignore)
}

// GenerateTestYAML generates test.yaml content.
func GenerateTestYAML(config *Config) string {
	agents := config.Agents

	// Determine artifact path
	artifactPath := "/artifacts"
	if config.ArtifactLevel == "uc" {
		artifactPath = "/uc-artifacts"
	}

	// Check if this is flat script mode (--filter was used)
	isFlatScriptMode := config.Filter != ""

	// Separate by type
	var tsAgents, pyAgents []AgentInfo
	for _, a := range agents {
		if a.AgentType == "typescript" {
			tsAgents = append(tsAgents, a)
		} else {
			pyAgents = append(pyAgents, a)
		}
	}

	// Build pre_run
	var preRunLines []string
	if len(pyAgents) > 0 {
		preRunLines = append(preRunLines, `  - routine: global.setup_for_python_agent
    params:
      meshctl_version: "${config.packages.cli_version}"
      mcpmesh_version: "${config.packages.sdk_python_version}"`)
	}
	if len(tsAgents) > 0 {
		preRunLines = append(preRunLines, `  - routine: global.setup_for_typescript_agent
    params:
      meshctl_version: "${config.packages.cli_version}"`)
	}
	preRun := "  # No setup routines needed"
	if len(preRunLines) > 0 {
		preRun = strings.Join(preRunLines, "\n")
	}

	// Build copy commands and paths for start
	var copyCommands []string
	var installSteps []string
	var startSteps []string

	if isFlatScriptMode {
		// Flat script mode: all scripts in one directory
		dirName := filepath.Base(config.FlatScriptDir)
		copyCommands = append(copyCommands, fmt.Sprintf("      cp -r %s/%s /workspace/", artifactPath, dirName))

		// No install steps for flat scripts (standalone)

		// Start each script
		for _, agent := range agents {
			startSteps = append(startSteps, fmt.Sprintf(`  - name: "Start %s"
    handler: shell
    command: "meshctl start %s/%s -d"
    workdir: /workspace`, agent.Name, dirName, agent.EntryPoint))
		}

		// Single wait after all starts
		startSteps = append(startSteps, fmt.Sprintf(`
  - name: "Wait for agents to register"
    handler: wait
    seconds: %d`, 5+len(agents)*2)) // Base 5s + 2s per agent
	} else {
		// Standard mode: each agent is its own directory
		for _, a := range agents {
			copyCommands = append(copyCommands, fmt.Sprintf("      cp -r %s/%s /workspace/", artifactPath, a.Name))
		}

		// Build install steps
		for _, agent := range agents {
			if agent.AgentType == "typescript" {
				installSteps = append(installSteps, fmt.Sprintf(`  - name: "Install %s dependencies"
    handler: npm-install
    path: /workspace/%s`, agent.Name, agent.Name))
			} else if agent.HasRequirements {
				installSteps = append(installSteps, fmt.Sprintf(`  - name: "Install %s dependencies"
    handler: pip-install
    path: /workspace/%s`, agent.Name, agent.Name))
			}
		}

		// Build start steps with wait per agent
		for _, agent := range agents {
			startSteps = append(startSteps, fmt.Sprintf(`  - name: "Start %s"
    handler: shell
    command: "meshctl start %s/%s -d"
    workdir: /workspace

  - name: "Wait for %s to register"
    handler: wait
    seconds: 8`, agent.Name, agent.Name, agent.EntryPoint, agent.Name))
		}
	}

	// Build assertions
	var assertions []string
	for _, agent := range agents {
		assertions = append(assertions, fmt.Sprintf(`  - expr: "${captured.agent_list} contains '%s'"
    message: "%s should be registered"`, agent.Name, agent.Name))
	}

	// Agent names for header
	var agentNames []string
	for _, a := range agents {
		agentNames = append(agentNames, a.Name)
	}

	// Test name
	testName := config.TestName
	if testName == "" {
		testName = strings.ReplaceAll(config.TCName, "_", " ")
		testName = strings.ReplaceAll(testName, "tc", "Test")
		testName = strings.Title(testName)
	}

	return fmt.Sprintf(`# Test Case: %s
# Auto-generated by tsuite scaffold
# Agents: %s

name: "%s"
description: "TODO: Add description"
tags:
  - scaffold
  - TODO
timeout: 300

pre_run:
%s

test:
  # Copy artifacts to workspace
  - name: "Copy artifacts to workspace"
    handler: shell
    command: |
%s
    capture: copy_output

%s

%s

  # Verify agents registered
  - name: "Verify agents registered"
    handler: shell
    command: "meshctl list"
    workdir: /workspace
    capture: agent_list

  # === TODO: Add your test steps below ===
  #
  # Example: Call a tool and capture result
  # - name: "Call calculate tool"
  #   handler: shell
  #   command: 'meshctl call calculate ''{"a": 10, "b": 5, "operator": "+"}'''
  #   workdir: /workspace
  #   capture: calculate_result

assertions:
%s

  # === Assertion Examples ===
  #
  # String contains:
  # - expr: "${captured.agent_list} contains 'my-agent'"
  #   message: "my-agent should be registered"
  #
  # JSON extraction with jq (numeric comparison - no quotes):
  # - expr: ${jq:captured.calculate_result:.content[0].text | fromjson | .result} == 15
  #   message: "Result should be 15"
  #
  # JSON extraction with jq (string comparison - with quotes):
  # - expr: "${jq:captured.calculate_result:.content[0].text | fromjson | .status} == 'success'"
  #   message: "Status should be success"

post_run:
  - handler: shell
    command: "meshctl stop 2>/dev/null || true"
    workdir: /workspace
    ignore_errors: true
  - routine: global.cleanup_workspace
`,
		config.TCName,
		strings.Join(agentNames, ", "),
		testName,
		preRun,
		strings.Join(copyCommands, "\n"),
		strings.Join(installSteps, "\n\n"),
		strings.Join(startSteps, "\n\n"),
		strings.Join(assertions, "\n\n"),
	)
}

// Run executes the scaffold operation.
func Run(config *Config) error {
	suitePath := config.SuitePath
	suitesDir := filepath.Join(suitePath, "suites")
	ucDir := filepath.Join(suitesDir, config.UCName)
	tcDir := filepath.Join(ucDir, config.TCName)

	// Determine artifacts directory
	artifactsDir := filepath.Join(tcDir, "artifacts")
	if config.ArtifactLevel == "uc" {
		artifactsDir = filepath.Join(ucDir, "artifacts")
	}

	// Check for TC conflict
	if _, err := os.Stat(tcDir); err == nil && !config.Force {
		return fmt.Errorf("test case already exists: %s\nUse --force to overwrite", tcDir)
	}

	// Check for artifact conflicts
	isFlatScriptMode := config.Filter != ""
	if !config.SkipArtifactCopy {
		if _, err := os.Stat(artifactsDir); err == nil {
			if isFlatScriptMode {
				// Flat script mode: check for the directory name
				dirName := filepath.Base(config.FlatScriptDir)
				target := filepath.Join(artifactsDir, dirName)
				if _, err := os.Stat(target); err == nil && !config.Force {
					return fmt.Errorf("artifact conflict - %s already exists\nUse --force to overwrite", dirName)
				}
			} else {
				// Standard mode: check each agent
				for _, agent := range config.Agents {
					target := filepath.Join(artifactsDir, agent.Name)
					if _, err := os.Stat(target); err == nil && !config.Force {
						return fmt.Errorf("artifact conflict - %s already exists\nUse --force to overwrite", agent.Name)
					}
				}
			}
		}
	}

	if config.DryRun {
		fmt.Println("\nDry run - no files will be created")
	}

	// Create UC if needed
	if _, err := os.Stat(ucDir); os.IsNotExist(err) {
		if config.DryRun {
			fmt.Printf("Would create UC: %s\n", ucDir)
		} else {
			os.MkdirAll(ucDir, 0755)
			fmt.Printf("✓ Created UC: %s/\n", config.UCName)
		}
	}

	// Create TC
	if _, err := os.Stat(tcDir); os.IsNotExist(err) {
		if config.DryRun {
			fmt.Printf("Would create TC: %s\n", tcDir)
		} else {
			os.MkdirAll(tcDir, 0755)
			fmt.Printf("✓ Created TC: %s/%s/\n", config.UCName, config.TCName)
		}
	} else if config.Force {
		fmt.Printf("! Overwriting TC: %s/%s/\n", config.UCName, config.TCName)
	}

	// Copy or symlink artifacts
	if !config.SkipArtifactCopy {
		if !config.DryRun {
			os.MkdirAll(artifactsDir, 0755)
		}

		if isFlatScriptMode {
			// Flat script mode: copy/symlink the whole directory once
			dirName := filepath.Base(config.FlatScriptDir)
			targetPath := filepath.Join(artifactsDir, dirName)

			if config.UseSymlinks {
				fmt.Println("✓ Creating artifact symlink:")
				absPath, _ := filepath.Abs(config.FlatScriptDir)
				if config.DryRun {
					fmt.Printf("  Would symlink: %s → %s\n", targetPath, absPath)
				} else {
					os.RemoveAll(targetPath)
					if err := os.Symlink(absPath, targetPath); err != nil {
						return fmt.Errorf("failed to create symlink: %w", err)
					}
					fmt.Printf("    - %s → %s (%d scripts)\n", dirName, absPath, len(config.Agents))
				}
			} else {
				fmt.Println("✓ Copying artifacts:")
				if config.DryRun {
					fmt.Printf("  Would copy: %s → %s\n", config.FlatScriptDir, targetPath)
				} else {
					os.RemoveAll(targetPath)
					os.MkdirAll(targetPath, 0755)
					// Only copy files matching the filter pattern
					for _, agent := range config.Agents {
						srcFile := filepath.Join(config.FlatScriptDir, agent.EntryPoint)
						dstFile := filepath.Join(targetPath, agent.EntryPoint)
						if err := copyFile(srcFile, dstFile); err != nil {
							return fmt.Errorf("failed to copy %s: %w", agent.EntryPoint, err)
						}
					}
					fmt.Printf("    - %s (%d scripts)\n", dirName, len(config.Agents))
				}
			}

			// List discovered scripts
			fmt.Println("  Scripts:")
			for _, agent := range config.Agents {
				fmt.Printf("    - %s\n", agent.EntryPoint)
			}
		} else {
			// Standard mode: copy/symlink each agent directory
			if config.UseSymlinks {
				fmt.Println("✓ Creating artifact symlinks:")
				for _, agent := range config.Agents {
					_, err := symlinkAgentToArtifacts(&agent, artifactsDir, config.DryRun)
					if err != nil {
						return fmt.Errorf("failed to create symlink for %s: %w", agent.Name, err)
					}
					typeLabel := "Python"
					if agent.AgentType == "typescript" {
						typeLabel = "TypeScript"
					}
					fmt.Printf("    - %s → %s (%s)\n", agent.Name, agent.Path, typeLabel)
				}
			} else {
				fmt.Println("✓ Copying artifacts:")
				for _, agent := range config.Agents {
					copyAgentToArtifacts(&agent, artifactsDir, config.DryRun)
					typeLabel := "Python"
					if agent.AgentType == "typescript" {
						typeLabel = "TypeScript"
					}
					fmt.Printf("    - %s (%s)\n", agent.Name, typeLabel)
				}
			}
		}
	} else {
		fmt.Println("! Skipping artifact copy (--skip-artifact-copy)")
	}

	// Generate test.yaml
	testYAMLPath := filepath.Join(tcDir, "test.yaml")
	testYAMLContent := GenerateTestYAML(config)

	if config.DryRun {
		fmt.Printf("\nWould create: %s\n", testYAMLPath)
		fmt.Println("\nGenerated test.yaml:")
		fmt.Println(testYAMLContent)
	} else {
		if err := os.WriteFile(testYAMLPath, []byte(testYAMLContent), 0644); err != nil {
			return fmt.Errorf("failed to write test.yaml: %w", err)
		}
		fmt.Printf("✓ Generated: %s/%s/test.yaml\n", config.UCName, config.TCName)
	}

	// Print completion message
	if !config.DryRun {
		fmt.Println("\nScaffold complete!")
		fmt.Println("\nNext steps:")
		fmt.Printf("  1. Edit test.yaml to add your test steps and assertions\n")
		fmt.Printf("  2. Run with: tsuite run --suite %s --tc %s/%s\n", config.SuitePath, config.UCName, config.TCName)
	}

	return nil
}
