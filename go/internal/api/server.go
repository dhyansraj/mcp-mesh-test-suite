// Package api provides the REST API server for the tsuite dashboard.
package api

import (
	"fmt"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/db"
)

// Server represents the API server
type Server struct {
	router *gin.Engine
	repo   *db.Repository
	port   int
	sseHub *SSEHub
}

// NewServer creates a new API server
func NewServer(port int) (*Server, error) {
	repo, err := db.NewRepository()
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	// Set Gin to release mode for cleaner output
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())

	// CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: true,
	}))

	s := &Server{
		router: router,
		repo:   repo,
		port:   port,
		sseHub: NewSSEHub(),
	}

	s.setupRoutes()

	return s, nil
}

// Run starts the server
func (s *Server) Run() error {
	addr := fmt.Sprintf(":%d", s.port)
	fmt.Printf("Starting API server on http://localhost%s\n", addr)
	return s.router.Run(addr)
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/health", s.healthCheck)

	// API routes
	api := s.router.Group("/api")
	{
		// Suites
		api.GET("/suites", s.listSuites)
		api.POST("/suites", s.createSuite)
		api.GET("/suites/:id", s.getSuite)
		api.PUT("/suites/:id", s.updateSuite)
		api.DELETE("/suites/:id", s.deleteSuite)
		api.POST("/suites/:id/sync", s.syncSuite)
		api.GET("/suites/:id/config", s.getSuiteConfig)
		api.PUT("/suites/:id/config", s.updateSuiteConfig)
		api.POST("/suites/:id/run", s.runSuite) // Launch tests from dashboard

		// Suite tests listing
		api.GET("/suites/:id/tests", s.getSuiteTests)

		// Test Case YAML Editor (Gin-friendly routes)
		api.GET("/suites/:id/test-yaml/*test_id", s.getTestYAMLHandler)
		api.PUT("/suites/:id/test-yaml/*test_id", s.updateTestYAMLHandler)
		api.GET("/suites/:id/test-steps/*test_id", s.getTestStepsHandler)
		api.PUT("/suites/:id/test-step/:phase/:index/*test_id", s.updateTestStepHandler)
		api.POST("/suites/:id/test-step/:phase/*test_id", s.addTestStepHandler)
		api.DELETE("/suites/:id/test-step/:phase/:index/*test_id", s.deleteTestStepHandler)

		// Runs
		api.GET("/runs", s.listRuns)
		api.POST("/runs", s.createRun)
		api.GET("/runs/latest", s.getLatestRun)
		api.GET("/runs/:run_id", s.getRun)
		api.PATCH("/runs/:run_id", s.updateRunStatus)
		api.GET("/runs/:run_id/tests", s.getRunTests)
		api.GET("/runs/:run_id/tests/tree", s.getRunTestsTree)              // Dashboard uses this
		api.GET("/runs/:run_id/tests/:test_id", s.getTestDetailByNumericID)  // Dashboard uses numeric ID
		api.GET("/runs/:run_id/test/*test_id", s.getTestDetail)              // CLI uses path-based ID
		api.PATCH("/runs/:run_id/test/*test_id", s.updateTestStatus)          // Go runner uses wildcard path
		api.PATCH("/runs/:run_id/tests/*test_id", s.updateTestStatusByPath)  // Python runner uses this (also wildcard for paths with /)
		api.POST("/runs/:run_id/complete", s.completeRun)
		api.POST("/runs/:run_id/cancel", s.cancelRun)
		api.POST("/runs/:run_id/rerun", s.rerunTests)
		api.DELETE("/runs/:run_id", s.deleteRun)

		// SSE Events
		api.GET("/events", s.streamEvents)
		api.POST("/events/emit", s.emitEvent) // For CLI to send events
		api.GET("/runs/:run_id/stream", s.streamRunEvents)

		// File Browser
		api.GET("/browse", s.browseFolders)
	}

	// Dashboard static files (must be after API routes)
	s.SetupDashboardRoutes()
}

// healthCheck handles GET /health
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
