package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// ==================== Suite Config ====================

// getSuiteConfig handles GET /api/suites/:id/config
func (s *Server) getSuiteConfig(c *gin.Context) {
	suite, ok := s.getSuiteByIDParam(c)
	if !ok {
		return
	}

	configPath := filepath.Join(suite.FolderPath, "config.yaml")

	// Check if config.yaml exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Config not found: " + configPath})
		return
	}

	// Read raw content
	rawYAML, err := os.ReadFile(configPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read config: " + err.Error()})
		return
	}

	// Parse YAML into structure
	var structure map[string]any
	if err := yaml.Unmarshal(rawYAML, &structure); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse config: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"suite_id":  suite.ID,
		"path":      configPath,
		"raw_yaml":  string(rawYAML),
		"structure": structure,
	})
}

// updateSuiteConfig handles PUT /api/suites/:id/config
func (s *Server) updateSuiteConfig(c *gin.Context) {
	suite, ok := s.getSuiteByIDParam(c)
	if !ok {
		return
	}

	configPath := filepath.Join(suite.FolderPath, "config.yaml")

	// Check if config.yaml exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Config not found: " + configPath})
		return
	}

	var req struct {
		Updates map[string]any `json:"updates"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.Updates == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Must provide 'updates'"})
		return
	}

	// Use YAMLDocument to preserve comments and key ordering
	doc, err := LoadYAMLFile(configPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read config: " + err.Error()})
		return
	}

	// Merge updates preserving structure
	if err := doc.MergeUpdates(req.Updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to merge updates: " + err.Error()})
		return
	}

	// Write back
	newYAML, err := doc.ToBytes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal config: " + err.Error()})
		return
	}

	if err := os.WriteFile(configPath, newYAML, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write config: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"raw_yaml": string(newYAML),
	})
}

// mergeUpdates recursively merges updates into the config
func mergeUpdates(config map[string]any, updates map[string]any) {
	for key, value := range updates {
		// Handle "__DELETE__" marker
		if strVal, ok := value.(string); ok && strVal == "__DELETE__" {
			delete(config, key)
			continue
		}

		// If both are maps, merge recursively
		if existingMap, ok := config[key].(map[string]any); ok {
			if updateMap, ok := value.(map[string]any); ok {
				mergeUpdates(existingMap, updateMap)
				continue
			}
		}

		// Otherwise, replace
		config[key] = value
	}
}

// ==================== Test Case YAML Editor ====================

// getTestYAMLHandler handles GET /api/suites/:id/test-yaml/*test_id
func (s *Server) getTestYAMLHandler(c *gin.Context) {
	id, ok := s.parseSuiteID(c)
	if !ok {
		return
	}
	testID := stripLeadingSlash(c.Param("test_id"))
	s.getTestYAML(c, id, testID)
}

// updateTestYAMLHandler handles PUT /api/suites/:id/test-yaml/*test_id
func (s *Server) updateTestYAMLHandler(c *gin.Context) {
	id, ok := s.parseSuiteID(c)
	if !ok {
		return
	}
	testID := stripLeadingSlash(c.Param("test_id"))
	s.updateTestYAML(c, id, testID)
}

// getTestStepsHandler handles GET /api/suites/:id/test-steps/*test_id
func (s *Server) getTestStepsHandler(c *gin.Context) {
	id, ok := s.parseSuiteID(c)
	if !ok {
		return
	}
	testID := stripLeadingSlash(c.Param("test_id"))
	s.getTestSteps(c, id, testID)
}

// updateTestStepHandler handles PUT /api/suites/:id/test-step/:phase/:index/*test_id
func (s *Server) updateTestStepHandler(c *gin.Context) {
	id, ok := s.parseSuiteID(c)
	if !ok {
		return
	}
	phase := c.Param("phase")
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid index"})
		return
	}
	testID := stripLeadingSlash(c.Param("test_id"))
	s.updateTestStep(c, id, testID, phase, index)
}

// addTestStepHandler handles POST /api/suites/:id/test-step/:phase/*test_id
func (s *Server) addTestStepHandler(c *gin.Context) {
	id, ok := s.parseSuiteID(c)
	if !ok {
		return
	}
	phase := c.Param("phase")
	testID := stripLeadingSlash(c.Param("test_id"))
	s.addTestStep(c, id, testID, phase)
}

// deleteTestStepHandler handles DELETE /api/suites/:id/test-step/:phase/:index/*test_id
func (s *Server) deleteTestStepHandler(c *gin.Context) {
	id, ok := s.parseSuiteID(c)
	if !ok {
		return
	}
	phase := c.Param("phase")
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid index"})
		return
	}
	testID := stripLeadingSlash(c.Param("test_id"))
	s.deleteTestStep(c, id, testID, phase, index)
}

// getTestYAML returns test YAML content
func (s *Server) getTestYAML(c *gin.Context, suiteID int64, testID string) {
	suite, ok := s.getSuiteOrError(c, suiteID)
	if !ok {
		return
	}

	// Build path to test.yaml
	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	// Check if test.yaml exists
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	// Read raw content
	rawYAML, err := os.ReadFile(testPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
		return
	}

	// Parse YAML into structure
	var structure map[string]any
	if err := yaml.Unmarshal(rawYAML, &structure); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse test: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"suite_id":  suiteID,
		"test_id":   testID,
		"path":      testPath,
		"raw_yaml":  string(rawYAML),
		"structure": structure,
	})
}

// updateTestYAML updates test YAML content (preserves comments and key ordering)
func (s *Server) updateTestYAML(c *gin.Context, suiteID int64, testID string) {
	suite, ok := s.getSuiteOrError(c, suiteID)
	if !ok {
		return
	}

	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	// Check if test.yaml exists
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	var req struct {
		RawYAML string         `json:"raw_yaml"`
		Updates map[string]any `json:"updates"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var newYAML []byte

	if req.RawYAML != "" {
		// Write raw YAML directly
		// Validate it's valid YAML first
		var test map[string]any
		if err := yaml.Unmarshal([]byte(req.RawYAML), &test); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid YAML: " + err.Error()})
			return
		}
		newYAML = []byte(req.RawYAML)
	} else if req.Updates != nil {
		// Use YAMLDocument to preserve comments and key ordering
		doc, err := LoadYAMLFile(testPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
			return
		}

		if err := doc.MergeUpdates(req.Updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to merge updates: " + err.Error()})
			return
		}

		newYAML, err = doc.ToBytes()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal test: " + err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Must provide 'raw_yaml' or 'updates'"})
		return
	}

	if err := os.WriteFile(testPath, newYAML, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write test: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"test_id":  testID,
		"raw_yaml": string(newYAML),
	})
}

// getTestSteps returns test steps
func (s *Server) getTestSteps(c *gin.Context, suiteID int64, testID string) {
	suite, ok := s.getSuiteOrError(c, suiteID)
	if !ok {
		return
	}

	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	rawYAML, err := os.ReadFile(testPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
		return
	}

	var structure map[string]any
	if err := yaml.Unmarshal(rawYAML, &structure); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse test: " + err.Error()})
		return
	}

	// Extract step arrays (return empty arrays if not present)
	preRun, _ := structure["pre_run"].([]any)
	test, _ := structure["test"].([]any)
	postRun, _ := structure["post_run"].([]any)
	assertions, _ := structure["assertions"].([]any)

	if preRun == nil {
		preRun = []any{}
	}
	if test == nil {
		test = []any{}
	}
	if postRun == nil {
		postRun = []any{}
	}
	if assertions == nil {
		assertions = []any{}
	}

	c.JSON(http.StatusOK, gin.H{
		"test_id":    testID,
		"pre_run":    preRun,
		"test":       test,
		"post_run":   postRun,
		"assertions": assertions,
	})
}

// updateTestStep updates a single step (preserves comments and key ordering)
func (s *Server) updateTestStep(c *gin.Context, suiteID int64, testID, phase string, index int) {
	if !validatePhase(c, phase) {
		return
	}

	suite, ok := s.getSuiteOrError(c, suiteID)
	if !ok {
		return
	}

	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Use YAMLDocument to preserve comments and key ordering
	doc, err := LoadYAMLFile(testPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
		return
	}

	// Update the step at the given index
	if err := doc.UpdateSequenceItem(phase, index, updates); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Save back
	newYAML, err := doc.ToBytes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal test: " + err.Error()})
		return
	}

	if err := os.WriteFile(testPath, newYAML, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write test: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// addTestStep adds a new step
func (s *Server) addTestStep(c *gin.Context, suiteID int64, testID, phase string) {
	if !validatePhase(c, phase) {
		return
	}

	suite, ok := s.getSuiteOrError(c, suiteID)
	if !ok {
		return
	}

	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	var req struct {
		Step  map[string]any `json:"step"`
		Index *int           `json:"index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.Step == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "step is required"})
		return
	}

	// Use YAMLDocument to preserve comments and key ordering
	doc, err := LoadYAMLFile(testPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
		return
	}

	// Add step at index or append
	if err := doc.AddSequenceItem(phase, req.Step, req.Index); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add step: " + err.Error()})
		return
	}

	// Save back
	newYAML, err := doc.ToBytes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal test: " + err.Error()})
		return
	}

	if err := os.WriteFile(testPath, newYAML, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write test: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// deleteTestStep deletes a step
func (s *Server) deleteTestStep(c *gin.Context, suiteID int64, testID, phase string, index int) {
	if !validatePhase(c, phase) {
		return
	}

	suite, ok := s.getSuiteOrError(c, suiteID)
	if !ok {
		return
	}

	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	// Use YAMLDocument to preserve comments and key ordering
	doc, err := LoadYAMLFile(testPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
		return
	}

	// Remove the step at index
	if err := doc.RemoveSequenceItem(phase, index); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Save back
	newYAML, err := doc.ToBytes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal test: " + err.Error()})
		return
	}

	if err := os.WriteFile(testPath, newYAML, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write test: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
