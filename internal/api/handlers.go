package api

import (
	"database/sql"

	"github.com/google/uuid"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/models"
)

// generateUUID creates a new UUID string
func generateUUID() string {
	return uuid.New().String()
}

// ==================== Shared Types ====================

// useCaseGroup represents a group of tests organized by use case
type useCaseGroup struct {
	UseCase string              `json:"use_case"`
	Tests   []models.TestResult `json:"tests"`
	Pending int                 `json:"pending"`
	Running int                 `json:"running"`
	Passed  int                 `json:"passed"`
	Failed  int                 `json:"failed"`
	Crashed int                 `json:"crashed"`
	Skipped int                 `json:"skipped"`
	Total   int                 `json:"total"`
}

// ==================== Null Value Helpers ====================

func nullStringValue(ns sql.NullString) any {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func nullInt64Value(ni sql.NullInt64) any {
	if ni.Valid {
		return ni.Int64
	}
	return nil
}
