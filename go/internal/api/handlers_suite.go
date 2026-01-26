package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/models"
)

// ==================== Suites ====================

// listSuites handles GET /api/suites
func (s *Server) listSuites(c *gin.Context) {
	suites, err := s.repo.GetAllSuites()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure suites is an empty array, not null
	if suites == nil {
		suites = []models.Suite{}
	}

	c.JSON(http.StatusOK, gin.H{
		"suites": suites,
		"count":  len(suites),
	})
}

// getSuite handles GET /api/suites/:id
func (s *Server) getSuite(c *gin.Context) {
	suite, ok := s.getSuiteByIDParam(c)
	if !ok {
		return
	}

	// Discover tests from filesystem
	tests, useCases, _ := DiscoverTests(suite.FolderPath)

	c.JSON(http.StatusOK, gin.H{
		"id":             suite.ID,
		"folder_path":    suite.FolderPath,
		"suite_name":     suite.SuiteName,
		"mode":           suite.Mode,
		"test_count":     suite.TestCount,
		"last_synced_at": suite.LastSyncedAt,
		"created_at":     suite.CreatedAt,
		"updated_at":     suite.UpdatedAt,
		"tests":          tests,
		"use_cases":      useCases,
	})
}

// createSuite handles POST /api/suites
func (s *Server) createSuite(c *gin.Context) {
	var req struct {
		FolderPath string `json:"folder_path"`
		Mode       string `json:"mode"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.FolderPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "folder_path is required"})
		return
	}

	// Normalize and expand path
	folderPath := req.FolderPath
	if len(folderPath) > 0 && folderPath[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			folderPath = filepath.Join(home, folderPath[1:])
		}
	}
	folderPath, _ = filepath.Abs(folderPath)

	// Check if directory exists
	info, err := os.Stat(folderPath)
	if os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Directory not found: " + folderPath})
		return
	}
	if !info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not a directory: " + folderPath})
		return
	}

	// Check for config.yaml
	configPath := filepath.Join(folderPath, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No config.yaml found in " + folderPath})
		return
	}

	// Check if suite already exists
	existing, err := s.repo.GetSuiteByPath(folderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Suite already exists", "suite": existing})
		return
	}

	// Parse config.yaml
	configData, err := os.ReadFile(configPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read config: " + err.Error()})
		return
	}

	var config map[string]any
	if err := yaml.Unmarshal(configData, &config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse config: " + err.Error()})
		return
	}

	// Extract suite info from config
	suiteConfig, _ := config["suite"].(map[string]any)
	suiteName := filepath.Base(folderPath)
	mode := "docker"
	if suiteConfig != nil {
		if n, ok := suiteConfig["name"].(string); ok && n != "" {
			suiteName = n
		}
		if m, ok := suiteConfig["mode"].(string); ok && m != "" {
			mode = m
		}
	}

	// Override mode if provided in request
	if req.Mode != "" {
		if req.Mode != "docker" && req.Mode != "standalone" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mode: " + req.Mode + ". Must be 'docker' or 'standalone'"})
			return
		}
		mode = req.Mode
	}

	// Discover tests using existing function
	tests, useCases, err := DiscoverTests(folderPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to discover tests: " + err.Error()})
		return
	}

	// Marshal config to JSON
	configJSON, _ := json.Marshal(config)

	// Create suite
	now := time.Now()
	suite := &models.Suite{
		FolderPath:   folderPath,
		SuiteName:    suiteName,
		Mode:         models.SuiteMode(mode),
		ConfigJSON:   sql.NullString{String: string(configJSON), Valid: true},
		TestCount:    len(tests),
		LastSyncedAt: &now,
	}

	if err := s.repo.CreateSuite(suite); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":          suite.ID,
		"folder_path": suite.FolderPath,
		"suite_name":  suite.SuiteName,
		"mode":        suite.Mode,
		"test_count":  suite.TestCount,
		"tests":       tests,
		"use_cases":   useCases,
	})
}

// updateSuite handles PUT /api/suites/:id
func (s *Server) updateSuite(c *gin.Context) {
	suite, ok := s.getSuiteByIDParam(c)
	if !ok {
		return
	}

	var req struct {
		SuiteName string `json:"suite_name"`
		Mode      string `json:"mode"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.SuiteName != "" {
		suite.SuiteName = req.SuiteName
	}
	if req.Mode != "" {
		suite.Mode = models.SuiteMode(req.Mode)
	}

	if err := s.repo.UpdateSuite(suite); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, suite)
}

// deleteSuite handles DELETE /api/suites/:id
func (s *Server) deleteSuite(c *gin.Context) {
	suite, ok := s.getSuiteByIDParam(c)
	if !ok {
		return
	}

	if err := s.repo.DeleteSuite(suite.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true, "id": suite.ID})
}

// syncSuite handles POST /api/suites/:id/sync
// Re-reads config.yaml and updates the cached config in the database
func (s *Server) syncSuite(c *gin.Context) {
	suite, ok := s.getSuiteByIDParam(c)
	if !ok {
		return
	}

	// Check if directory still exists
	if _, err := os.Stat(suite.FolderPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Directory not found: " + suite.FolderPath})
		return
	}

	// Parse config.yaml
	configPath := filepath.Join(suite.FolderPath, "config.yaml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read config: " + err.Error()})
		return
	}

	var config map[string]any
	if err := yaml.Unmarshal(configData, &config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse config: " + err.Error()})
		return
	}

	// Extract suite info from config
	suiteConfig, _ := config["suite"].(map[string]any)
	suiteName := suite.SuiteName // Keep existing name as default
	mode := string(suite.Mode)   // Keep existing mode as default
	if suiteConfig != nil {
		if n, ok := suiteConfig["name"].(string); ok && n != "" {
			suiteName = n
		}
		if m, ok := suiteConfig["mode"].(string); ok && m != "" {
			mode = m
		}
	}

	// Discover tests
	tests, _, err := DiscoverTests(suite.FolderPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to discover tests: " + err.Error()})
		return
	}

	// Marshal config to JSON
	configJSON, _ := json.Marshal(config)

	// Update suite in database
	now := time.Now()
	suite.SuiteName = suiteName
	suite.Mode = models.SuiteMode(mode)
	suite.ConfigJSON = sql.NullString{String: string(configJSON), Valid: true}
	suite.TestCount = len(tests)
	suite.LastSyncedAt = &now

	if err := s.repo.UpdateSuite(suite); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, suite)
}

// getSuiteTests handles GET /api/suites/:id/tests
func (s *Server) getSuiteTests(c *gin.Context) {
	suite, ok := s.getSuiteByIDParam(c)
	if !ok {
		return
	}

	// Discover tests from filesystem
	tests, _, err := DiscoverTests(suite.FolderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Apply filters
	ucFilter := c.Query("uc")
	tagFilter := c.Query("tag")

	if ucFilter != "" || tagFilter != "" {
		filtered := make([]TestInfo, 0)
		for _, t := range tests {
			if ucFilter != "" && t.UseCase != ucFilter {
				continue
			}
			if tagFilter != "" {
				hasTag := false
				for _, tag := range t.Tags {
					if tag == tagFilter {
						hasTag = true
						break
					}
				}
				if !hasTag {
					continue
				}
			}
			filtered = append(filtered, t)
		}
		tests = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"suite_id": suite.ID,
		"tests":    tests,
		"count":    len(tests),
	})
}
