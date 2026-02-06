package api

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/db"
	"github.com/gin-gonic/gin"
)

// ==================== Secrets Management ====================

// listSecrets handles GET /api/secrets
// Returns all secrets with masked values
func (s *Server) listSecrets(c *gin.Context) {
	secrets, err := s.repo.GetAllSecrets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure we return [] not null for empty list
	if secrets == nil {
		secrets = []db.Secret{}
	}

	// Mask values for display
	for i := range secrets {
		secrets[i].Value = "***"
	}

	c.JSON(http.StatusOK, secrets)
}

// createSecret handles POST /api/secrets
// Creates a new secret
func (s *Server) createSecret(c *gin.Context) {
	var req struct {
		Key   string `json:"key" binding:"required"`
		Value string `json:"value" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key and value are required"})
		return
	}

	// Validate key format (alphanumeric, underscores, no spaces)
	req.Key = strings.TrimSpace(req.Key)
	if req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key cannot be empty"})
		return
	}

	// Check if secret already exists
	existing, err := s.repo.GetSecret(req.Key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "secret with this key already exists"})
		return
	}

	secret, err := s.repo.CreateSecret(req.Key, req.Value)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Mask value in response
	secret.Value = "***"
	c.JSON(http.StatusCreated, secret)
}

// updateSecret handles PUT /api/secrets/:key
// Updates an existing secret
func (s *Server) updateSecret(c *gin.Context) {
	key := c.Param("key")

	var req struct {
		Value string `json:"value" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "value is required"})
		return
	}

	err := s.repo.UpdateSecret(key, req.Value)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "key": key})
}

// deleteSecret handles DELETE /api/secrets/:key
// Deletes a secret
func (s *Server) deleteSecret(c *gin.Context) {
	key := c.Param("key")

	err := s.repo.DeleteSecret(key)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "key": key})
}

// getSecretValues handles GET /api/secrets/values
// Returns actual secret values (for runner use)
func (s *Server) getSecretValues(c *gin.Context) {
	secrets, err := s.repo.GetSecretValues()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, secrets)
}
