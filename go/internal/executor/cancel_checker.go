// Package executor provides test execution helpers.
package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/client"
)

// CancelChecker polls the API for cancel requests and cancels the context when requested.
type CancelChecker struct {
	client     *client.Client
	runID      string
	cancelFunc context.CancelFunc
	interval   time.Duration
}

// NewCancelChecker creates a new cancel checker.
func NewCancelChecker(apiClient *client.Client, runID string, cancelFunc context.CancelFunc) *CancelChecker {
	return &CancelChecker{
		client:     apiClient,
		runID:      runID,
		cancelFunc: cancelFunc,
		interval:   2 * time.Second,
	}
}

// Start begins polling for cancel requests in a goroutine.
// The goroutine will exit when ctx is cancelled or when a cancel request is detected.
func (cc *CancelChecker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(cc.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if cc.runID != "" {
					if isCancelled, _ := cc.client.CheckCancelRequested(cc.runID); isCancelled {
						fmt.Println("\n[CANCEL] Cancel requested - terminating...")
						cc.cancelFunc()
						return
					}
				}
			}
		}
	}()
}

// StartCancelChecker is a convenience function that creates and starts a cancel checker.
// This is the most common usage pattern.
func StartCancelChecker(ctx context.Context, cancelFunc context.CancelFunc, apiClient *client.Client, runID string) {
	cc := NewCancelChecker(apiClient, runID, cancelFunc)
	cc.Start(ctx)
}
