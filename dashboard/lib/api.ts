/**
 * API client for tsuite backend
 */

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:9999";

export interface Run {
  run_id: string;
  started_at: string | null;
  finished_at: string | null;
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  total_tests: number;
  passed: number;
  failed: number;
  skipped: number;
  duration_ms: number | null;
  cli_version: string | null;
  docker_image: string | null;
}

export interface RunSummary extends Run {
  tests?: TestResult[];
}

export interface TestResult {
  id: number;
  run_id: string;
  test_id: string;
  use_case: string;
  test_case: string;
  name: string;
  status: "pending" | "running" | "passed" | "failed" | "skipped";
  started_at: string | null;
  finished_at: string | null;
  duration_ms: number | null;
  error_message: string | null;
  tags: string[];
}

export interface TestDetail extends TestResult {
  steps: StepResult[];
  assertions: AssertionResult[];
}

export interface StepResult {
  id: number;
  test_result_id: number;
  step_index: number;
  phase: string;
  handler: string | null;
  description: string | null;
  status: string;
  duration_ms: number | null;
  stdout: string | null;
  stderr: string | null;
  error_message: string | null;
}

export interface AssertionResult {
  id: number;
  test_result_id: number;
  assertion_index: number;
  expression: string;
  message: string | null;
  passed: boolean;
  actual_value: string | null;
  expected_value: string | null;
}

export interface Stats {
  total_runs: number;
  total_tests_executed: number;
  total_passed: number;
  total_failed: number;
  avg_run_duration_ms: number | null;
  pass_rate: number;
}

export interface Suite {
  id: number;
  folder_path: string;
  suite_name: string;
  mode: "docker" | "standalone";
  config_json: string | null;
  config: Record<string, unknown> | null;
  test_count: number;
  last_synced_at: string | null;
  created_at: string | null;
  updated_at: string | null;
}

export interface SuiteWithTests extends Suite {
  tests: SuiteTest[];
}

export interface SuiteTest {
  use_case: string;
  test_case: string;
  test_id: string;
  name: string | null;
  tags: string[];
}

export interface SuitesResponse {
  suites: Suite[];
  count: number;
}

export interface BrowseDirectory {
  name: string;
  path: string;
  is_suite: boolean;
}

export interface BrowseResponse {
  path: string;
  parent: string | null;
  directories: BrowseDirectory[];
  is_suite: boolean;
}

export interface RunsResponse {
  runs: Run[];
  count: number;
  limit: number;
  offset: number;
}

export interface TestsResponse {
  run_id: string;
  tests: TestResult[];
  count: number;
}

// API Functions

export async function getRuns(
  limit = 20,
  offset = 0
): Promise<RunsResponse> {
  const res = await fetch(
    `${API_BASE}/api/runs?limit=${limit}&offset=${offset}`,
    { cache: "no-store" }
  );
  if (!res.ok) throw new Error("Failed to fetch runs");
  return res.json();
}

export async function getLatestRun(): Promise<Run | null> {
  const res = await fetch(`${API_BASE}/api/runs/latest`, { cache: "no-store" });
  if (res.status === 404) return null;
  if (!res.ok) throw new Error("Failed to fetch latest run");
  return res.json();
}

export async function getRun(runId: string): Promise<RunSummary> {
  const res = await fetch(`${API_BASE}/api/runs/${runId}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch run");
  return res.json();
}

export async function getRunTests(
  runId: string,
  status?: string
): Promise<TestsResponse> {
  const url = status
    ? `${API_BASE}/api/runs/${runId}/tests?status=${status}`
    : `${API_BASE}/api/runs/${runId}/tests`;
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) throw new Error("Failed to fetch tests");
  return res.json();
}

export async function getTestDetail(
  runId: string,
  testId: number
): Promise<TestDetail> {
  const res = await fetch(`${API_BASE}/api/runs/${runId}/tests/${testId}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch test detail");
  return res.json();
}

export async function getStats(): Promise<Stats> {
  const res = await fetch(`${API_BASE}/api/stats`, { cache: "no-store" });
  if (!res.ok) throw new Error("Failed to fetch stats");
  return res.json();
}

export async function getFlakyTests(limit = 20): Promise<{ tests: unknown[]; count: number }> {
  const res = await fetch(`${API_BASE}/api/stats/flaky?limit=${limit}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch flaky tests");
  return res.json();
}

export async function getSlowestTests(limit = 10): Promise<{ tests: unknown[]; count: number }> {
  const res = await fetch(`${API_BASE}/api/stats/slowest?limit=${limit}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch slowest tests");
  return res.json();
}

// Suite API Functions

export async function getSuites(): Promise<SuitesResponse> {
  const res = await fetch(`${API_BASE}/api/suites`, { cache: "no-store" });
  if (!res.ok) throw new Error("Failed to fetch suites");
  return res.json();
}

export async function getSuite(suiteId: number): Promise<SuiteWithTests> {
  const res = await fetch(`${API_BASE}/api/suites/${suiteId}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch suite");
  return res.json();
}

export async function addSuite(folderPath: string): Promise<Suite> {
  const res = await fetch(`${API_BASE}/api/suites`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ folder_path: folderPath }),
  });
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to add suite");
  }
  return res.json();
}

export async function deleteSuite(suiteId: number): Promise<void> {
  const res = await fetch(`${API_BASE}/api/suites/${suiteId}`, {
    method: "DELETE",
  });
  if (!res.ok) throw new Error("Failed to delete suite");
}

export async function syncSuite(suiteId: number): Promise<Suite> {
  const res = await fetch(`${API_BASE}/api/suites/${suiteId}/sync`, {
    method: "POST",
  });
  if (!res.ok) throw new Error("Failed to sync suite");
  return res.json();
}

export async function browseFolders(path?: string): Promise<BrowseResponse> {
  const url = path
    ? `${API_BASE}/api/browse?path=${encodeURIComponent(path)}`
    : `${API_BASE}/api/browse`;
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to browse folders");
  }
  return res.json();
}

export interface RunResponse {
  started: boolean;
  pid: number;
  description: string;
  mode: string;
  command: string;
}

export async function runTests(
  suiteId: number,
  options?: { uc?: string; tc?: string }
): Promise<RunResponse> {
  const res = await fetch(`${API_BASE}/api/suites/${suiteId}/run`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(options || {}),
  });
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to start test run");
  }
  return res.json();
}

// Run Tree API Functions

export interface RunTestTreeUseCase {
  use_case: string;
  tests: TestResult[];
  passed: number;
  failed: number;
  pending: number;
  running: number;
}

export interface RunTestTreeResponse {
  run_id: string;
  use_cases: RunTestTreeUseCase[];
  total: number;
}

export async function getRunTestsTree(runId: string): Promise<RunTestTreeResponse> {
  const res = await fetch(`${API_BASE}/api/runs/${runId}/tests/tree`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch test tree");
  return res.json();
}

export interface RunExtended extends Run {
  pending_count: number;
  running_count: number;
  suite_id: number | null;
  filters: Record<string, unknown> | null;
  mode: string;
}

export async function getRunExtended(runId: string): Promise<RunExtended> {
  const res = await fetch(`${API_BASE}/api/runs/${runId}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch run");
  return res.json();
}

// Helper functions

export function formatDuration(ms: number | null): string {
  if (ms === null || ms === undefined) return "-";
  if (ms < 1000) return `${ms}ms`;
  const seconds = ms / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return `${minutes}m ${remainingSeconds.toFixed(0)}s`;
}

export function formatRelativeTime(dateString: string | null): string {
  if (!dateString) return "-";
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

export function getStatusColor(status: string): string {
  switch (status) {
    case "passed":
    case "completed":
      return "text-success";
    case "failed":
      return "text-destructive";
    case "running":
      return "text-primary";
    case "pending":
      return "text-muted-foreground";
    case "skipped":
    case "cancelled":
      return "text-warning";
    default:
      return "text-foreground";
  }
}

export function getStatusBgColor(status: string): string {
  switch (status) {
    case "passed":
    case "completed":
      return "bg-success/20 text-success";
    case "failed":
      return "bg-destructive/20 text-destructive";
    case "running":
      return "bg-primary/20 text-primary";
    case "pending":
      return "bg-muted text-muted-foreground";
    case "skipped":
    case "cancelled":
      return "bg-warning/20 text-warning";
    default:
      return "bg-muted text-foreground";
  }
}

// ============================================================================
// Test Case Editor API Functions
// ============================================================================

export interface TestCaseYaml {
  suite_id: number;
  test_id: string;
  path: string;
  raw_yaml: string;
  structure: TestCaseStructure;
}

export interface TestCaseStructure {
  name?: string;
  description?: string;
  tags?: string[];
  timeout?: number;
  pre_run?: TestStep[];
  test?: TestStep[];
  post_run?: TestStep[];
  assertions?: TestAssertion[];
}

export interface TestStep {
  name?: string;
  handler?: string;
  routine?: string;
  command?: string;
  params?: Record<string, unknown>;
  capture?: string;
  timeout?: number;
  workdir?: string;
  ignore_errors?: boolean;
  // Wait handler
  seconds?: number;
  type?: string;
  // HTTP handler
  method?: string;
  url?: string;
  headers?: Record<string, string>;
  body?: unknown;
  expect_status?: number;
}

export interface TestAssertion {
  expr: string;
  message?: string;
}

export interface TestStepsResponse {
  test_id: string;
  pre_run: TestStep[];
  test: TestStep[];
  post_run: TestStep[];
  assertions: TestAssertion[];
}

export async function getTestCaseYaml(
  suiteId: number,
  testId: string
): Promise<TestCaseYaml> {
  const res = await fetch(
    `${API_BASE}/api/suites/${suiteId}/tests/${testId}/yaml`,
    { cache: "no-store" }
  );
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to fetch test YAML");
  }
  return res.json();
}

export async function updateTestCaseYaml(
  suiteId: number,
  testId: string,
  options: { raw_yaml?: string; updates?: Partial<TestCaseStructure> }
): Promise<{ success: boolean; test_id: string; raw_yaml: string }> {
  const res = await fetch(
    `${API_BASE}/api/suites/${suiteId}/tests/${testId}/yaml`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(options),
    }
  );
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to update test YAML");
  }
  return res.json();
}

export async function getTestSteps(
  suiteId: number,
  testId: string
): Promise<TestStepsResponse> {
  const res = await fetch(
    `${API_BASE}/api/suites/${suiteId}/tests/${testId}/steps`,
    { cache: "no-store" }
  );
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to fetch test steps");
  }
  return res.json();
}

export async function updateTestStep(
  suiteId: number,
  testId: string,
  phase: "pre_run" | "test" | "post_run",
  index: number,
  step: Partial<TestStep>
): Promise<{ success: boolean }> {
  const res = await fetch(
    `${API_BASE}/api/suites/${suiteId}/tests/${testId}/steps/${phase}/${index}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(step),
    }
  );
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to update step");
  }
  return res.json();
}

export async function addTestStep(
  suiteId: number,
  testId: string,
  phase: "pre_run" | "test" | "post_run",
  step: TestStep,
  index?: number
): Promise<{ success: boolean }> {
  const res = await fetch(
    `${API_BASE}/api/suites/${suiteId}/tests/${testId}/steps/${phase}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ step, index }),
    }
  );
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to add step");
  }
  return res.json();
}

export async function deleteTestStep(
  suiteId: number,
  testId: string,
  phase: "pre_run" | "test" | "post_run",
  index: number
): Promise<{ success: boolean }> {
  const res = await fetch(
    `${API_BASE}/api/suites/${suiteId}/tests/${testId}/steps/${phase}/${index}`,
    { method: "DELETE" }
  );
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to delete step");
  }
  return res.json();
}

// ============================================================================
// Suite Config Editor API Functions
// ============================================================================

export interface SuiteConfigStructure {
  suite?: {
    name?: string;
    mode?: "docker" | "standalone";
  };
  packages?: {
    cli_version?: string;
    sdk_python_version?: string;
    sdk_typescript_version?: string;
  };
  docker?: {
    base_image?: string;
    network?: string;
  };
  defaults?: {
    timeout?: number;
    parallel?: number;
    retry?: number;
  };
  reports?: {
    output_dir?: string;
    formats?: string[];
    keep_last?: number;
  };
  aliases?: Record<string, string>;
}

export interface SuiteConfigResponse {
  suite_id: number;
  path: string;
  raw_yaml: string;
  structure: SuiteConfigStructure;
}

export async function getSuiteConfig(
  suiteId: number
): Promise<SuiteConfigResponse> {
  const res = await fetch(`${API_BASE}/api/suites/${suiteId}/config`, {
    cache: "no-store",
  });
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to fetch config");
  }
  return res.json();
}

export async function updateSuiteConfig(
  suiteId: number,
  updates: Partial<SuiteConfigStructure>
): Promise<{ success: boolean; suite_id: number; raw_yaml: string }> {
  const res = await fetch(`${API_BASE}/api/suites/${suiteId}/config`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ updates }),
  });
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || "Failed to update config");
  }
  return res.json();
}
