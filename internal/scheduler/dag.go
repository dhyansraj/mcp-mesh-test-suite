package scheduler

import (
	"fmt"
	"strings"
	"sync"
)

// NodeState represents the execution state of a test node
type NodeState int

const (
	NodePending NodeState = iota
	NodeRunning
	NodePassed
	NodeFailed
	NodeSkipped
)

// TestNode represents a test in the dependency graph
type TestNode struct {
	TestID     string
	DependsOn  []string   // parent test IDs (must complete before this runs)
	Dependents []string   // child test IDs (depend on this test)
	State      NodeState
	SkipReason string
}

// DAG holds the dependency graph of tests
type DAG struct {
	mu    sync.Mutex
	nodes map[string]*TestNode
}

// Build constructs a DAG from a list of test IDs and a function that returns
// dependencies for each test. Short references (no "/") are resolved relative
// to the test's use case.
func Build(testIDs []string, loadDeps func(testID string) []string) (*DAG, error) {
	dag := &DAG{
		nodes: make(map[string]*TestNode),
	}

	// Create nodes for all tests
	for _, id := range testIDs {
		dag.nodes[id] = &TestNode{
			TestID: id,
			State:  NodePending,
		}
	}

	// Resolve dependencies and build edges
	for _, id := range testIDs {
		deps := loadDeps(id)
		if len(deps) == 0 {
			continue
		}

		node := dag.nodes[id]
		uc := ""
		if idx := strings.Index(id, "/"); idx >= 0 {
			uc = id[:idx]
		}

		for _, dep := range deps {
			// Resolve short references
			resolved := dep
			if !strings.Contains(dep, "/") && uc != "" {
				resolved = uc + "/" + dep
			}

			parent, exists := dag.nodes[resolved]
			if !exists {
				return nil, fmt.Errorf("test %q depends on %q which is not in the test set", id, resolved)
			}

			node.DependsOn = append(node.DependsOn, resolved)
			parent.Dependents = append(parent.Dependents, id)
		}
	}

	return dag, nil
}

// Validate checks the DAG for cycles using DFS with coloring.
func (d *DAG) Validate() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 0=white (unvisited), 1=gray (in progress), 2=black (done)
	color := make(map[string]int)
	for id := range d.nodes {
		color[id] = 0
	}

	var dfs func(id string, path []string) error
	dfs = func(id string, path []string) error {
		color[id] = 1 // gray
		node := d.nodes[id]
		for _, dep := range node.Dependents {
			if color[dep] == 1 {
				// Found a cycle
				cycle := append(path, id, dep)
				return fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> "))
			}
			if color[dep] == 0 {
				if err := dfs(dep, append(path, id)); err != nil {
					return err
				}
			}
		}
		color[id] = 2 // black
		return nil
	}

	for id := range d.nodes {
		if color[id] == 0 {
			if err := dfs(id, nil); err != nil {
				return err
			}
		}
	}

	return nil
}

// HasDependencies returns true if any test in the DAG has dependencies.
func (d *DAG) HasDependencies() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, node := range d.nodes {
		if len(node.DependsOn) > 0 {
			return true
		}
	}
	return false
}

// DependentCount returns how many tests have at least one dependency.
func (d *DAG) DependentCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	count := 0
	for _, node := range d.nodes {
		if len(node.DependsOn) > 0 {
			count++
		}
	}
	return count
}

// ReadyTests returns test IDs that are pending and have all dependencies satisfied
// (all parents are in Passed state, or have no parents).
func (d *DAG) ReadyTests() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	var ready []string
	for _, node := range d.nodes {
		if node.State != NodePending {
			continue
		}
		allSatisfied := true
		for _, dep := range node.DependsOn {
			parent := d.nodes[dep]
			if parent.State != NodePassed {
				allSatisfied = false
				break
			}
		}
		if allSatisfied {
			ready = append(ready, node.TestID)
		}
	}
	return ready
}

// MarkRunning marks a test as currently executing.
func (d *DAG) MarkRunning(testID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if node, ok := d.nodes[testID]; ok {
		node.State = NodeRunning
	}
}

// MarkPassed marks a test as passed. Returns nil.
func (d *DAG) MarkPassed(testID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if node, ok := d.nodes[testID]; ok {
		node.State = NodePassed
	}
}

// MarkFailed marks a test as failed and recursively skips all transitive dependents.
// Returns the list of test IDs that were skipped as a result.
func (d *DAG) MarkFailed(testID string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	node := d.nodes[testID]
	if node == nil {
		return nil
	}
	node.State = NodeFailed

	// Recursively skip dependents
	var skipped []string
	var skipDeps func(id, reason string)
	skipDeps = func(id, reason string) {
		for _, depID := range d.nodes[id].Dependents {
			dep := d.nodes[depID]
			if dep.State == NodePending {
				dep.State = NodeSkipped
				dep.SkipReason = reason
				skipped = append(skipped, depID)
				skipDeps(depID, reason)
			}
		}
	}
	skipDeps(testID, "dependency failed: "+testID)
	return skipped
}

// MarkSkipped marks a test as skipped with a reason and propagates to dependents.
func (d *DAG) MarkSkipped(testID, reason string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	node := d.nodes[testID]
	if node == nil {
		return nil
	}
	node.State = NodeSkipped
	node.SkipReason = reason

	var skipped []string
	var skipDeps func(id, reason string)
	skipDeps = func(id, reason string) {
		for _, depID := range d.nodes[id].Dependents {
			dep := d.nodes[depID]
			if dep.State == NodePending {
				dep.State = NodeSkipped
				dep.SkipReason = reason
				skipped = append(skipped, depID)
				skipDeps(depID, reason)
			}
		}
	}
	skipDeps(testID, reason)
	return skipped
}

// AllDone returns true when every node is in a terminal state.
func (d *DAG) AllDone() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, node := range d.nodes {
		if node.State == NodePending || node.State == NodeRunning {
			return false
		}
	}
	return true
}

// Stats returns aggregate counts.
func (d *DAG) Stats() (passed, failed, skipped int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, node := range d.nodes {
		switch node.State {
		case NodePassed:
			passed++
		case NodeFailed:
			failed++
		case NodeSkipped:
			skipped++
		}
	}
	return
}

// GetNode returns a copy of the node for a test ID (for reading outside the lock).
func (d *DAG) GetNode(testID string) *TestNode {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n, ok := d.nodes[testID]; ok {
		cp := *n
		return &cp
	}
	return nil
}
