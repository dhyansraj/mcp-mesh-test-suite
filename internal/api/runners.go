package api

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RunnersFS holds the embedded runner binaries.
// Set by the main package if runner files are available.
var RunnersFS embed.FS

// RunnersPrefix is the subdirectory in the embed.FS
var RunnersPrefix = "runners"

// HasRunners is true if runner binaries are embedded
var HasRunners = false

// handleListRunners returns list of available runner files
// GET /api/runners
func (s *Server) handleListRunners(c *gin.Context) {
	if !HasRunners {
		c.JSON(http.StatusNotFound, gin.H{"error": "No runner binaries embedded. Build with: make build-with-runners"})
		return
	}

	subFS, err := fs.Sub(RunnersFS, RunnersPrefix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var files []gin.H
	entries, err := fs.ReadDir(subFS, ".")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip .gitkeep and .keep files
		if name == ".gitkeep" || name == ".keep" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, gin.H{
			"name": name,
			"size": info.Size(),
		})
	}

	c.JSON(http.StatusOK, gin.H{"runners": files})
}

// handleGetRunner serves a specific runner binary
// GET /api/runners/:name
func (s *Server) handleGetRunner(c *gin.Context) {
	if !HasRunners {
		c.JSON(http.StatusNotFound, gin.H{"error": "No runner binaries embedded"})
		return
	}

	name := c.Param("name")

	subFS, err := fs.Sub(RunnersFS, RunnersPrefix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	data, err := fs.ReadFile(subFS, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Runner not found: " + name})
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+name)
	c.Data(http.StatusOK, "application/octet-stream", data)
}
