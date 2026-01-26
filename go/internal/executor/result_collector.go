// Package executor provides test execution helpers.
package executor

import (
	"fmt"
	"sync"
	"time"
)

// TestResult represents the outcome of a single test execution.
type TestResult struct {
	TestID    string
	Passed    bool
	Error     string
	Duration  time.Duration
	Cancelled bool
}

// TestResults holds the aggregated test results.
type TestResults struct {
	Passed      int
	Failed      int
	Skipped     int
	FailedTests []string
	Cancelled   bool
}

// CollectResults reads from a result channel and aggregates test outcomes.
// It prints status messages for each test and returns aggregated results.
// The mutex is used to safely accumulate results when called concurrently.
func CollectResults(resultCh <-chan TestResult) TestResults {
	var results TestResults
	var mu sync.Mutex

	for result := range resultCh {
		mu.Lock()
		if result.Cancelled {
			fmt.Printf("[SKIP] %s (cancelled)\n", result.TestID)
			results.Skipped++
			results.Cancelled = true
		} else if result.Passed {
			fmt.Printf("[PASS] %s (%.1fs)\n", result.TestID, result.Duration.Seconds())
			results.Passed++
		} else {
			fmt.Printf("[FAIL] %s - %s (%.1fs)\n", result.TestID, result.Error, result.Duration.Seconds())
			results.Failed++
			results.FailedTests = append(results.FailedTests, result.TestID)
		}
		mu.Unlock()
	}
	return results
}
