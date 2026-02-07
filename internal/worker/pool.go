package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/client"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/executor"
)

// PoolConfig configures a worker pool run
type PoolConfig struct {
	Handler   WorkerHandler
	Tests     []string
	Workers   int
	APIURL    string
	RunID     string
	Timeout   time.Duration
	APIClient *client.Client // for cancel checking + metadata
}

// PoolResult holds aggregated results from a pool run
type PoolResult struct {
	Passed      int
	Failed      int
	Skipped     int
	FailedTests []string
	Cancelled   bool
}

// RunPool executes tests using the worker handler pattern with a concurrent worker pool
func RunPool(ctx context.Context, cancelFunc context.CancelFunc, cfg PoolConfig) PoolResult {
	testCh := make(chan string, len(cfg.Tests))
	resultCh := make(chan executor.TestResult, len(cfg.Tests))

	// Start cancel checker if API client is available
	if cfg.APIClient != nil {
		executor.StartCancelChecker(ctx, cancelFunc, cfg.APIClient, cfg.RunID)
	}

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for testID := range testCh {
				// Check if cancelled before starting test
				select {
				case <-ctx.Done():
					resultCh <- executor.TestResult{TestID: testID, Cancelled: true}
					continue
				default:
				}

				result := runSingleTest(ctx, cfg, testID)
				resultCh <- result
			}
		}(i)
	}

	// Feed tests to workers
	for _, t := range cfg.Tests {
		testCh <- t
	}
	close(testCh)

	// Wait for all workers, then close results
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	results := executor.CollectResults(resultCh)
	return PoolResult{
		Passed:      results.Passed,
		Failed:      results.Failed,
		Skipped:     results.Skipped,
		FailedTests: results.FailedTests,
		Cancelled:   results.Cancelled,
	}
}

// runSingleTest executes one test through the handler lifecycle
func runSingleTest(ctx context.Context, cfg PoolConfig, testID string) executor.TestResult {
	// Start worker
	info, err := cfg.Handler.StartWorker(ctx, testID, cfg.RunID, cfg.APIURL)
	if err != nil {
		// Report failure to API since runner never started
		if cfg.APIClient != nil && cfg.RunID != "" {
			cfg.APIClient.UpdateTestStatus(cfg.RunID, testID, &client.UpdateTestStatusRequest{
				Status:       "failed",
				ErrorMessage: fmt.Sprintf("failed to start worker: %v", err),
			})
		}
		return executor.TestResult{
			TestID: testID,
			Passed: false,
			Error:  fmt.Sprintf("failed to start worker: %v", err),
		}
	}

	// Report pod/node metadata for K8s mode
	if info.PodName != "" && cfg.APIClient != nil && cfg.RunID != "" {
		cfg.APIClient.UpdateTestMeta(cfg.RunID, testID, info.PodName, info.NodeName)
	}

	// Ensure cleanup always happens
	defer cfg.Handler.CleanupWorker(ctx, info)

	// Wait with timeout
	var waitCtx context.Context
	var waitCancel context.CancelFunc
	if cfg.Timeout > 0 {
		waitCtx, waitCancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		waitCtx, waitCancel = context.WithTimeout(ctx, 10*time.Minute)
	}
	defer waitCancel()

	result, err := cfg.Handler.WaitForWorker(waitCtx, info)
	if err != nil {
		// Check cancellation
		if ctx.Err() == context.Canceled {
			return executor.TestResult{TestID: testID, Cancelled: true}
		}
		return executor.TestResult{
			TestID: testID,
			Passed: false,
			Error:  err.Error(),
		}
	}

	return executor.TestResult{
		TestID:   testID,
		Passed:   result.Passed,
		Error:    result.Error,
		Duration: result.Duration,
	}
}
