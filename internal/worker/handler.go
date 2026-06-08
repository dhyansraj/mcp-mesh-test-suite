package worker

import (
	"context"
	"time"
)

// WorkerInfo describes a running worker (container, pod, or process)
type WorkerInfo struct {
	ID       string // container ID, pod name, or PID
	TestID   string
	PodName  string // k8s only
	NodeName string // k8s only
}

// WorkerResult holds the outcome of a worker execution
type WorkerResult struct {
	Passed   bool
	Error    string
	Duration time.Duration
	ImageID  string // k8s only: resolved container image digest
}

// WorkerHandler abstracts test execution across standalone, Docker, and K8s modes
type WorkerHandler interface {
	// StartWorker creates and starts a worker, returns immediately
	StartWorker(ctx context.Context, testID string, runID string, apiURL string) (WorkerInfo, error)
	// WaitForWorker blocks until worker completes
	WaitForWorker(ctx context.Context, info WorkerInfo) (*WorkerResult, error)
	// CleanupWorker removes the worker (idempotent)
	CleanupWorker(ctx context.Context, info WorkerInfo) error
	// Name returns handler name for logging
	Name() string
	// Close releases any shared resources
	Close() error
}
