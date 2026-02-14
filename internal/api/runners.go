package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

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
	if HasRunners {
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
		return
	}

	// No embedded runners — return fetchable runners list
	runners := listFetchableRunners()
	c.JSON(http.StatusOK, gin.H{"runners": runners})
}

// handleGetRunner serves a specific runner binary
// GET /api/runners/:name
func (s *Server) handleGetRunner(c *gin.Context) {
	name := c.Param("name")

	// Try embedded FS first
	if HasRunners {
		subFS, err := fs.Sub(RunnersFS, RunnersPrefix)
		if err == nil {
			if data, err := fs.ReadFile(subFS, name); err == nil {
				c.Header("Content-Disposition", "attachment; filename="+name)
				c.Data(http.StatusOK, "application/octet-stream", data)
				return
			}
		}
	}

	// Fallback to on-demand fetch
	data, err := fetchRunner(name)
	if err != nil {
		if strings.Contains(err.Error(), "unknown runner") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		}
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+name)
	c.Data(http.StatusOK, "application/octet-stream", data)
}
