package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/client"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/executor"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/worker"
)

// RunScheduled executes tests respecting dependency order from the DAG.
// If dag is nil, falls back to worker.RunPool (backward compatible).
func RunScheduled(ctx context.Context, cancelFunc context.CancelFunc, cfg worker.PoolConfig, dag *DAG) worker.PoolResult {
	// No dependencies — use existing pool
	if dag == nil || !dag.HasDependencies() {
		return worker.RunPool(ctx, cancelFunc, cfg)
	}

	// Start cancel checker
	if cfg.APIClient != nil {
		executor.StartCancelChecker(ctx, cancelFunc, cfg.APIClient, cfg.RunID)
	}

	resultCh := make(chan executor.TestResult, len(cfg.Tests))
	dispatched := 0
	collected := 0
	total := len(cfg.Tests)

	var result worker.PoolResult
	running := 0

	// Dispatch initial ready tests
	dispatchReady := func() {
		ready := dag.ReadyTests()
		for _, testID := range ready {
			if running >= cfg.Workers {
				break
			}
			dag.MarkRunning(testID)
			running++
			dispatched++
			go func(id string) {
				r := worker.RunSingleTest(ctx, cfg, id)
				resultCh <- r
			}(testID)
		}
	}

	// Initial dispatch
	dispatchReady()

	// If nothing was dispatched and we have tests, something is wrong
	if dispatched == 0 && total > 0 {
		return worker.PoolResult{
			Failed: total,
		}
	}

	// Wave loop: collect results, update DAG, dispatch newly ready tests
	for collected < total && !dag.AllDone() {
		select {
		case <-ctx.Done():
			result.Cancelled = true
			// Mark remaining pending as cancelled
			for _, node := range dag.ReadyTests() {
				dag.MarkSkipped(node, "run cancelled")
			}
			return result

		case tr := <-resultCh:
			collected++
			running--

			if tr.Cancelled {
				result.Cancelled = true
				dag.MarkSkipped(tr.TestID, "run cancelled")
			} else if tr.Passed {
				result.Passed++
				dag.MarkPassed(tr.TestID)
			} else {
				result.Failed++
				result.FailedTests = append(result.FailedTests, tr.TestID)

				// Skip dependents
				skipped := dag.MarkFailed(tr.TestID)
				for _, skippedID := range skipped {
					collected++
					result.Skipped++
					// Report skipped to API
					if cfg.APIClient != nil && cfg.RunID != "" {
						skipReason := fmt.Sprintf("dependency failed: %s", tr.TestID)
						cfg.APIClient.UpdateTestStatus(cfg.RunID, skippedID, &client.UpdateTestStatusRequest{
							Status:       "skipped",
							ErrorMessage: skipReason,
						})
					}
					fmt.Printf("[SKIP] %s (dependency failed: %s)\n", skippedID, tr.TestID)
				}
			}

			// Dispatch newly ready tests
			dispatchReady()
		}
	}

	// Drain any remaining results
	for running > 0 {
		select {
		case tr := <-resultCh:
			running--
			collected++
			if tr.Passed {
				result.Passed++
				dag.MarkPassed(tr.TestID)
			} else if tr.Cancelled {
				result.Cancelled = true
			} else {
				result.Failed++
				result.FailedTests = append(result.FailedTests, tr.TestID)
				skipped := dag.MarkFailed(tr.TestID)
				result.Skipped += len(skipped)
			}
		case <-time.After(10 * time.Minute):
			// Safety timeout
			return result
		}
	}

	return result
}
