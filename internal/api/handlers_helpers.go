package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/models"
)

// parseSuiteID extracts and validates suite ID from the request parameter.
// Returns the ID and true if successful, or sends error response and returns false.
func (s *Server) parseSuiteID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return 0, false
	}
	return id, true
}

// getSuiteOrError fetches a suite by ID and handles errors.
// Returns the suite and true if successful, or sends error response and returns false.
func (s *Server) getSuiteOrError(c *gin.Context, id int64) (*models.Suite, bool) {
	suite, err := s.repo.GetSuiteByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return nil, false
	}
	return suite, true
}

// getSuiteByIDParam is a convenience method that combines parseSuiteID and getSuiteOrError.
// Returns the suite and true if successful, or sends error response and returns false.
func (s *Server) getSuiteByIDParam(c *gin.Context) (*models.Suite, bool) {
	id, ok := s.parseSuiteID(c)
	if !ok {
		return nil, false
	}
	return s.getSuiteOrError(c, id)
}

// getRunOrError fetches a run by ID and handles errors.
// Returns the run and true if successful, or sends error response and returns false.
func (s *Server) getRunOrError(c *gin.Context, runID string) (*models.Run, bool) {
	run, err := s.repo.GetRunByID(runID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Run not found"})
		return nil, false
	}
	return run, true
}

// getRunByIDParam is a convenience method that extracts run_id param and fetches the run.
// Returns the run and true if successful, or sends error response and returns false.
func (s *Server) getRunByIDParam(c *gin.Context) (*models.Run, bool) {
	runID := c.Param("run_id")
	return s.getRunOrError(c, runID)
}

// validatePhase validates that a phase name is valid (pre_run, test, post_run).
// Returns true if valid, or sends error response and returns false.
func validatePhase(c *gin.Context, phase string) bool {
	if phase != "pre_run" && phase != "test" && phase != "post_run" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid phase: " + phase})
		return false
	}
	return true
}

// stripLeadingSlash removes the leading slash from a path parameter.
// Gin wildcard params include a leading slash.
func stripLeadingSlash(path string) string {
	return strings.TrimPrefix(path, "/")
}

// Phase constants
const (
	PhasePreRun  = "pre_run"
	PhaseTest    = "test"
	PhasePostRun = "post_run"
)

// Status constants for runs
const (
	RunStatusPending   = "pending"
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
	RunStatusCancelled = "cancelled"
)

// Status constants for tests
const (
	TestStatusPending = "pending"
	TestStatusRunning = "running"
	TestStatusPassed  = "passed"
	TestStatusFailed  = "failed"
	TestStatusSkipped = "skipped"
	TestStatusCrashed = "crashed"
)
