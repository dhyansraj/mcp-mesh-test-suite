package api

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// DashboardFS holds the embedded dashboard files.
// This will be set by the main package if dashboard files are available.
var DashboardFS embed.FS

// DashboardPrefix is the subdirectory in the embed.FS where dashboard files are stored
var DashboardPrefix = "dashboard"

// HasDashboard returns true if dashboard files are embedded
var HasDashboard = false

// SetupDashboardRoutes configures routes to serve the embedded dashboard
func (s *Server) SetupDashboardRoutes() {
	if !HasDashboard {
		// No dashboard embedded, serve a simple message at root
		s.router.GET("/", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message": "tsuite API server",
				"api":     "/api",
				"health":  "/health",
				"note":    "Dashboard not embedded. Run with dashboard build to serve UI.",
			})
		})
		return
	}

	// Create a sub-filesystem rooted at the dashboard directory
	subFS, err := fs.Sub(DashboardFS, DashboardPrefix)
	if err != nil {
		// Fallback if sub-filesystem fails
		s.router.GET("/", func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to load dashboard: " + err.Error(),
			})
		})
		return
	}

	// Serve index.html at root
	s.router.GET("/", func(c *gin.Context) {
		serveDashboardFile(c, subFS, "index.html")
	})

	// Serve static files and handle SPA routing
	s.router.NoRoute(func(c *gin.Context) {
		urlPath := c.Request.URL.Path

		// Skip API routes - they should 404 normally
		if strings.HasPrefix(urlPath, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			return
		}

		// Skip health check
		if urlPath == "/health" {
			return
		}

		// Clean the path
		cleanPath := strings.TrimPrefix(urlPath, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		// Try to serve the exact file
		if serveIfExists(c, subFS, cleanPath) {
			return
		}

		// Try with .html extension (Next.js static export pattern)
		if serveIfExists(c, subFS, cleanPath+".html") {
			return
		}

		// Try as directory with index.html
		if serveIfExists(c, subFS, path.Join(cleanPath, "index.html")) {
			return
		}

		// Fallback to index.html for SPA client-side routing
		serveDashboardFile(c, subFS, "index.html")
	})
}

// serveIfExists tries to serve a file if it exists, returns true if served
func serveIfExists(c *gin.Context, fsys fs.FS, filePath string) bool {
	file, err := fsys.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		return false
	}

	serveDashboardFile(c, fsys, filePath)
	return true
}

// serveDashboardFile serves a file from the embedded filesystem
func serveDashboardFile(c *gin.Context, fsys fs.FS, filePath string) {
	data, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Set content type based on extension
	contentType := getContentType(filePath)

	// Set cache headers for static assets (JS, CSS, fonts, images)
	// HTML files should not be cached to ensure fresh content
	ext := strings.ToLower(path.Ext(filePath))
	if ext == ".js" || ext == ".css" || ext == ".woff" || ext == ".woff2" ||
		ext == ".ttf" || ext == ".png" || ext == ".jpg" || ext == ".svg" || ext == ".ico" {
		// Cache static assets for 1 year (Next.js uses content hashes)
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else if ext == ".html" {
		// Don't cache HTML
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	}

	c.Data(http.StatusOK, contentType, data)
}

// getContentType returns the content type based on file extension
func getContentType(filePath string) string {
	ext := strings.ToLower(path.Ext(filePath))
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".eot":
		return "application/vnd.ms-fontobject"
	case ".map":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
