package api

import (
	"encoding/json"
	"sync"
	"time"
)

// SSEEvent represents an SSE event to be broadcast
type SSEEvent struct {
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

// NewSSEEvent creates a new SSE event with timestamp
func NewSSEEvent(eventType string, data map[string]any) *SSEEvent {
	return &SSEEvent{
		Type:      eventType,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data:      data,
	}
}

// ToSSE formats the event as an SSE message
func (e *SSEEvent) ToSSE() string {
	payload := map[string]any{
		"type":      e.Type,
		"timestamp": e.Timestamp,
	}
	for k, v := range e.Data {
		payload[k] = v
	}
	jsonBytes, _ := json.Marshal(payload)
	return "data: " + string(jsonBytes) + "\n\n"
}

// SSEHub manages SSE subscriptions and event broadcasting
type SSEHub struct {
	mu sync.RWMutex

	// Global subscribers (receive all events)
	globalSubscribers map[chan string]bool

	// Per-run subscribers
	runSubscribers map[string]map[chan string]bool

	// Current run being executed
	currentRunID string

	// Event cache for late subscribers (per run)
	eventCache map[string][]string

	// Max events to cache per run
	maxCacheSize int
}

// NewSSEHub creates a new SSE hub
func NewSSEHub() *SSEHub {
	return &SSEHub{
		globalSubscribers: make(map[chan string]bool),
		runSubscribers:    make(map[string]map[chan string]bool),
		eventCache:        make(map[string][]string),
		maxCacheSize:      100,
	}
}

// SubscribeGlobal adds a subscriber to the global event stream
func (h *SSEHub) SubscribeGlobal() chan string {
	ch := make(chan string, 100) // Buffered to avoid blocking
	h.mu.Lock()
	h.globalSubscribers[ch] = true
	h.mu.Unlock()
	return ch
}

// UnsubscribeGlobal removes a subscriber from the global stream
func (h *SSEHub) UnsubscribeGlobal(ch chan string) {
	h.mu.Lock()
	delete(h.globalSubscribers, ch)
	h.mu.Unlock()
	close(ch)
}

// SubscribeRun adds a subscriber to a specific run's event stream
func (h *SSEHub) SubscribeRun(runID string) chan string {
	ch := make(chan string, 100)
	h.mu.Lock()
	if h.runSubscribers[runID] == nil {
		h.runSubscribers[runID] = make(map[chan string]bool)
	}
	h.runSubscribers[runID][ch] = true
	h.mu.Unlock()
	return ch
}

// UnsubscribeRun removes a subscriber from a run's stream
func (h *SSEHub) UnsubscribeRun(runID string, ch chan string) {
	h.mu.Lock()
	if subs := h.runSubscribers[runID]; subs != nil {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(h.runSubscribers, runID)
		}
	}
	h.mu.Unlock()
	close(ch)
}

// Emit broadcasts an event to all relevant subscribers
func (h *SSEHub) Emit(event *SSEEvent, runID string) {
	sseData := event.ToSSE()

	h.mu.Lock()
	defer h.mu.Unlock()

	// Cache event for late subscribers
	if runID != "" {
		h.eventCache[runID] = append(h.eventCache[runID], sseData)
		if len(h.eventCache[runID]) > h.maxCacheSize {
			h.eventCache[runID] = h.eventCache[runID][len(h.eventCache[runID])-h.maxCacheSize:]
		}
	}

	// Send to run-specific subscribers
	if runID != "" {
		if subs := h.runSubscribers[runID]; subs != nil {
			for ch := range subs {
				select {
				case ch <- sseData:
				default:
					// Channel full, skip (non-blocking)
				}
			}
		}
	}

	// Send to global subscribers
	for ch := range h.globalSubscribers {
		select {
		case ch <- sseData:
		default:
			// Channel full, skip (non-blocking)
		}
	}
}

// SetCurrentRun sets the current run ID
func (h *SSEHub) SetCurrentRun(runID string) {
	h.mu.Lock()
	h.currentRunID = runID
	h.mu.Unlock()
}

// GetCurrentRun returns the current run ID
func (h *SSEHub) GetCurrentRun() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.currentRunID
}

// GetCachedEvents returns cached events for a run
func (h *SSEHub) GetCachedEvents(runID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if events, ok := h.eventCache[runID]; ok {
		result := make([]string, len(events))
		copy(result, events)
		return result
	}
	return nil
}

// ClearCache clears the event cache for a run
func (h *SSEHub) ClearCache(runID string) {
	h.mu.Lock()
	delete(h.eventCache, runID)
	h.mu.Unlock()
}

// Convenience methods for emitting specific event types

// EmitRunStarted broadcasts a run_started event
func (h *SSEHub) EmitRunStarted(runID string, totalTests int) {
	h.SetCurrentRun(runID)
	h.Emit(NewSSEEvent("run_started", map[string]any{
		"run_id":      runID,
		"total_tests": totalTests,
	}), runID)
}

// EmitTestStarted broadcasts a test_started event
func (h *SSEHub) EmitTestStarted(runID, testID, name string) {
	h.Emit(NewSSEEvent("test_started", map[string]any{
		"run_id":  runID,
		"test_id": testID,
		"name":    name,
	}), runID)
}

// EmitTestCompleted broadcasts a test_completed event
func (h *SSEHub) EmitTestCompleted(runID, testID, status string, durationMS int64, stepsPassed, stepsFailed int) {
	h.Emit(NewSSEEvent("test_completed", map[string]any{
		"run_id":       runID,
		"test_id":      testID,
		"status":       status,
		"duration_ms":  durationMS,
		"steps_passed": stepsPassed,
		"steps_failed": stepsFailed,
	}), runID)
}

// EmitRunCompleted broadcasts a run_completed event
func (h *SSEHub) EmitRunCompleted(runID string, passed, failed, skipped int, durationMS int64) {
	h.Emit(NewSSEEvent("run_completed", map[string]any{
		"run_id":      runID,
		"passed":      passed,
		"failed":      failed,
		"skipped":     skipped,
		"duration_ms": durationMS,
	}), runID)
	h.SetCurrentRun("")
}

// EmitCancelRequested broadcasts a cancel_requested event
func (h *SSEHub) EmitCancelRequested(runID string) {
	h.Emit(NewSSEEvent("cancel_requested", map[string]any{
		"run_id": runID,
	}), runID)
}

// EmitRunCancelled broadcasts a run_cancelled event (after CLI terminates workers)
func (h *SSEHub) EmitRunCancelled(runID string, passed, failed, skipped int, durationMS int64) {
	h.Emit(NewSSEEvent("run_cancelled", map[string]any{
		"run_id":      runID,
		"passed":      passed,
		"failed":      failed,
		"skipped":     skipped,
		"duration_ms": durationMS,
	}), runID)
	h.SetCurrentRun("")
}
