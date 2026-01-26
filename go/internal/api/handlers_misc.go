package api

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ==================== Stats ====================

// getStats handles GET /api/stats
func (s *Server) getStats(c *gin.Context) {
	stats, err := s.repo.GetRunStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// ==================== Suite Run ====================

// runSuite handles POST /api/suites/:id/run
// Launches the Go CLI as a subprocess to run tests
func (s *Server) runSuite(c *gin.Context) {
	suite, ok := s.getSuiteByIDParam(c)
	if !ok {
		return
	}

	// Check if suite folder exists
	if _, err := os.Stat(suite.FolderPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Directory not found: " + suite.FolderPath})
		return
	}

	// Parse request body for filters
	var req struct {
		UC       string   `json:"uc"`
		TC       string   `json:"tc"`
		Tags     []string `json:"tags"`
		SkipTags []string `json:"skip_tags"`
	}
	c.ShouldBindJSON(&req) // Optional body

	// Find tsuite executable
	execPath, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find executable: " + err.Error()})
		return
	}

	// Build CLI command
	apiURL := "http://localhost:" + strconv.Itoa(s.port)
	args := []string{
		"run",
		"--suite-path", suite.FolderPath,
		"--api-url", apiURL,
	}

	// Add filter flags
	if req.TC != "" {
		args = append(args, "--tc", req.TC)
	} else if req.UC != "" {
		args = append(args, "--uc", req.UC)
	}
	// If no filters, run all tests (default behavior)

	// Add tag filters
	for _, tag := range req.Tags {
		args = append(args, "--tags", tag)
	}
	// Note: skip_tags not implemented in Go CLI yet

	// Create log file
	logFile, err := os.CreateTemp("", "tsuite_run_*.log")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create log file: " + err.Error()})
		return
	}
	logPath := logFile.Name()

	// Start subprocess
	cmd := newExecCommand(execPath, args...)
	cmd.Dir = suite.FolderPath
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start CLI: " + err.Error()})
		return
	}

	// Close log file - subprocess has inherited the FD
	logFile.Close()

	// Build description
	var description string
	if req.TC != "" {
		description = "Running test case: " + req.TC
	} else if req.UC != "" {
		description = "Running use case: " + req.UC
	} else {
		description = "Running all tests in: " + suite.SuiteName
	}

	// Don't wait for subprocess - let it run independently
	go func() {
		cmd.Wait()
	}()

	c.JSON(http.StatusOK, gin.H{
		"started":     true,
		"pid":         cmd.Process.Pid,
		"description": description,
		"log_file":    logPath,
	})
}

// newExecCommand creates a new exec.Cmd - extracted for testing
var newExecCommand = func(name string, args ...string) *execCmd {
	cmd := &execCmd{Cmd: exec.Command(name, args...)}
	return cmd
}

// execCmd wraps exec.Cmd for easier testing
type execCmd struct {
	*exec.Cmd
}

// ==================== File Browser ====================

// browseFolders handles GET /api/browse
// Browse directories for folder selection in the dashboard
func (s *Server) browseFolders(c *gin.Context) {
	requestedPath := c.Query("path")

	var path string
	if requestedPath == "" {
		// Default to home directory
		home, err := os.UserHomeDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot get home directory"})
			return
		}
		path = home
	} else {
		// Expand ~ if present
		if len(requestedPath) > 0 && requestedPath[0] == '~' {
			home, err := os.UserHomeDir()
			if err == nil {
				requestedPath = filepath.Join(home, requestedPath[1:])
			}
		}
		// Resolve to absolute path
		absPath, err := filepath.Abs(requestedPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}
		path = absPath
	}

	// Security: don't allow browsing certain system directories
	restrictedPrefixes := []string{"/proc", "/sys", "/dev", "/etc/shadow"}
	for _, prefix := range restrictedPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
	}

	// Check if path exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Path does not exist: " + path})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not a directory: " + path})
		return
	}

	// Check if this is a valid test suite
	configPath := filepath.Join(path, "config.yaml")
	suitesPath := filepath.Join(path, "suites")
	_, configErr := os.Stat(configPath)
	suitesInfo, suitesErr := os.Stat(suitesPath)
	isSuite := configErr == nil && suitesErr == nil && suitesInfo.IsDir()

	// Get parent directory
	parent := filepath.Dir(path)
	var parentPtr *string
	if parent != path {
		parentPtr = &parent
	}

	// List directories
	type DirEntry struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		IsSuite bool   `json:"is_suite"`
	}
	var directories []DirEntry

	entries, err := os.ReadDir(path)
	if os.IsPermission(err) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}

		entryPath := filepath.Join(path, entry.Name())

		// Check if subdir is a suite
		subConfigPath := filepath.Join(entryPath, "config.yaml")
		subSuitesPath := filepath.Join(entryPath, "suites")
		_, subConfigErr := os.Stat(subConfigPath)
		subSuitesInfo, subSuitesErr := os.Stat(subSuitesPath)
		subdirIsSuite := subConfigErr == nil && subSuitesErr == nil && subSuitesInfo.IsDir()

		directories = append(directories, DirEntry{
			Name:    entry.Name(),
			Path:    entryPath,
			IsSuite: subdirIsSuite,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"path":        path,
		"parent":      parentPtr,
		"directories": directories,
		"is_suite":    isSuite,
	})
}
