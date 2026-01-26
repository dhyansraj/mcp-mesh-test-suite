package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ==================== SSE Events ====================

// streamEvents handles GET /api/events
func (s *Server) streamEvents(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Subscribe to global events
	eventCh := s.sseHub.SubscribeGlobal()
	defer s.sseHub.UnsubscribeGlobal(eventCh)

	// Get current running run
	currentRunID := s.sseHub.GetCurrentRun()
	if currentRunID == "" {
		// Try to find from DB
		runningRun, _ := s.repo.GetRunningRun()
		if runningRun != nil {
			currentRunID = runningRun.RunID
		}
	}

	// Send connected event
	connected := map[string]any{
		"type":           "connected",
		"current_run_id": currentRunID,
	}
	connectedJSON, _ := json.Marshal(connected)
	c.Writer.WriteString("data: " + string(connectedJSON) + "\n\n")
	c.Writer.Flush()

	// Send cached events for current run (for late subscribers)
	if currentRunID != "" {
		cachedEvents := s.sseHub.GetCachedEvents(currentRunID)
		for _, event := range cachedEvents {
			c.Writer.WriteString(event)
			c.Writer.Flush()
		}
	}

	// Keep connection alive with heartbeat and stream events
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	clientGone := c.Request.Context().Done()

	for {
		select {
		case <-clientGone:
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			c.Writer.WriteString(event)
			c.Writer.Flush()
		case <-ticker.C:
			// Send heartbeat
			c.Writer.WriteString(": heartbeat\n\n")
			c.Writer.Flush()
		}
	}
}

// streamRunEvents handles GET /api/runs/:run_id/stream
func (s *Server) streamRunEvents(c *gin.Context) {
	runID := c.Param("run_id")

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Subscribe to run-specific events
	eventCh := s.sseHub.SubscribeRun(runID)
	defer s.sseHub.UnsubscribeRun(runID, eventCh)

	// Send initial state
	run, err := s.repo.GetRunByID(runID)
	if err == nil && run != nil {
		tests, _ := s.repo.GetTestResultsByRunID(runID)
		initial := map[string]any{
			"type":   "initial_state",
			"run_id": runID,
			"run":    run,
			"tests":  tests,
		}
		initialJSON, _ := json.Marshal(initial)
		c.Writer.WriteString("data: " + string(initialJSON) + "\n\n")
	}

	// Send connected event
	connected := map[string]any{
		"type":   "connected",
		"run_id": runID,
	}
	connectedJSON, _ := json.Marshal(connected)
	c.Writer.WriteString("data: " + string(connectedJSON) + "\n\n")
	c.Writer.Flush()

	// Send cached events for this run
	cachedEvents := s.sseHub.GetCachedEvents(runID)
	for _, event := range cachedEvents {
		c.Writer.WriteString(event)
		c.Writer.Flush()
	}

	// Keep connection alive with heartbeat and stream events
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	clientGone := c.Request.Context().Done()

	for {
		select {
		case <-clientGone:
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			c.Writer.WriteString(event)
			c.Writer.Flush()
		case <-ticker.C:
			// Send heartbeat
			c.Writer.WriteString(": heartbeat\n\n")
			c.Writer.Flush()
		}
	}
}

// emitEvent handles POST /api/events/emit
// This endpoint receives events from CLI subprocesses and broadcasts via SSE
func (s *Server) emitEvent(c *gin.Context) {
	var req map[string]any

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	eventType, _ := req["type"].(string)
	if eventType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'type' field"})
		return
	}

	runID, _ := req["run_id"].(string)

	// Create and emit the event
	event := NewSSEEvent(eventType, req)
	s.sseHub.Emit(event, runID)

	// Update current run tracking
	if eventType == "run_started" {
		s.sseHub.SetCurrentRun(runID)
	} else if eventType == "run_completed" {
		s.sseHub.SetCurrentRun("")
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
