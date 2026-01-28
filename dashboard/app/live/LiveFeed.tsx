"use client";

import { useMemo, useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useLiveEvents, SSEEvent } from "@/lib/sse";
import {
  formatDuration,
  getRunExtended,
  getRunTestsTree,
  getTestDetail,
  cancelRun,
  rerunFromRun,
  runTests,
  RunExtended,
  RunTestTreeResponse,
  TestResult,
  TestDetail,
  StepResult,
  AssertionResult,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  CheckCircle,
  XCircle,
  Clock,
  Wifi,
  WifiOff,
  ChevronRight,
  ChevronDown,
  Loader2,
  Circle,
  AlertCircle,
  FolderOpen,
  Folder,
  Terminal,
  FileText,
  StopCircle,
  Play,
  RotateCcw,
} from "lucide-react";
import { cn } from "@/lib/utils";
import SlotCounter from "react-slot-counter";

// ============================================================================
// StatsCards Component
// ============================================================================

interface StatsCardsProps {
  pending: number;
  running: number;
  passed: number;
  failed: number;
  onFilterClick?: (status: string) => void;
  activeFilter?: string | null;
}

function StatsCards({
  pending,
  running,
  passed,
  failed,
  onFilterClick,
  activeFilter,
}: StatsCardsProps) {
  const cards = [
    {
      label: "Pending",
      value: pending,
      icon: Circle,
      color: "text-muted-foreground",
      bgColor: "bg-muted/50",
      status: "pending",
    },
    {
      label: "Running",
      value: running,
      icon: Loader2,
      color: "text-primary",
      bgColor: "bg-primary/10",
      status: "running",
      iconClass: running > 0 ? "animate-spin" : "",
    },
    {
      label: "Passed",
      value: passed,
      icon: CheckCircle,
      color: "text-success",
      bgColor: "bg-success/10",
      status: "passed",
    },
    {
      label: "Failed",
      value: failed,
      icon: XCircle,
      color: "text-destructive",
      bgColor: "bg-destructive/10",
      status: "failed",
    },
  ];

  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
      {cards.map((card) => {
        const Icon = card.icon;
        const isActive = activeFilter === card.status;
        return (
          <Card
            key={card.status}
            className={cn(
              "cursor-pointer transition-all hover:scale-105",
              isActive && "ring-2 ring-primary"
            )}
            onClick={() => onFilterClick?.(isActive ? "" : card.status)}
          >
            <CardContent className="p-4">
              <div className="flex items-start justify-between">
                <div className="flex flex-col">
                  <div className={cn("p-2 rounded-lg w-fit", card.bgColor)}>
                    <Icon
                      className={cn("h-5 w-5", card.color, card.iconClass)}
                    />
                  </div>
                  <p className="mt-2 text-sm text-muted-foreground">{card.label}</p>
                </div>
                <span className={cn("text-5xl font-bold leading-none", card.color)}>
                  <SlotCounter value={card.value} sequentialAnimationMode />
                </span>
              </div>
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}

// ============================================================================
// ProgressBar Component
// ============================================================================

interface ProgressBarProps {
  completed: number;
  total: number;
  passed: number;
  failed: number;
}

function ProgressBarSection({ completed, total, passed, failed }: ProgressBarProps) {
  const percentage = total > 0 ? Math.round((completed / total) * 100) : 0;

  return (
    <Card>
      <CardContent className="p-4">
        <div className="flex justify-between text-sm mb-2">
          <span className="text-muted-foreground">Progress</span>
          <span className="font-medium">
            {percentage}% ({completed}/{total})
          </span>
        </div>
        <div className="h-3 rounded-full bg-muted overflow-hidden">
          <div className="h-full flex transition-all duration-500">
            <div
              className="bg-success transition-all duration-500"
              style={{ width: `${total > 0 ? (passed / total) * 100 : 0}%` }}
            />
            <div
              className="bg-destructive transition-all duration-500"
              style={{ width: `${total > 0 ? (failed / total) * 100 : 0}%` }}
            />
          </div>
        </div>
        <div className="flex justify-between text-xs mt-2 text-muted-foreground">
          <span className="text-success">{passed} passed</span>
          <span className="text-destructive">{failed} failed</span>
        </div>
      </CardContent>
    </Card>
  );
}

// ============================================================================
// CurrentlyRunning Component
// ============================================================================

interface CurrentlyRunningProps {
  tests: TestResult[];
  getElapsed: (test: TestResult) => number;
}

function CurrentlyRunning({ tests, getElapsed }: CurrentlyRunningProps) {
  if (tests.length === 0) {
    return (
      <Card className="border-dashed">
        <CardContent className="flex items-center justify-center py-8 text-muted-foreground">
          <Clock className="mr-2 h-5 w-5" />
          <span>Waiting for test execution...</span>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="border-primary bg-primary/5">
      <CardHeader className="pb-2">
        <CardTitle className="flex items-center gap-2 text-base">
          <Loader2 className="h-5 w-5 animate-spin text-primary" />
          Currently Running ({tests.length})
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          {tests.map((test) => (
            <div key={test.test_id} className="flex items-center justify-between border-b border-primary/20 pb-2 last:border-0 last:pb-0">
              <div className="min-w-0 flex-1">
                <p className="font-medium truncate">{test.name || test.test_id}</p>
                <p className="text-sm text-muted-foreground font-mono truncate">{test.test_id}</p>
              </div>
              <p className="font-mono text-sm text-primary ml-4">
                {formatDuration(getElapsed(test))}
              </p>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

// ============================================================================
// TestTree Component
// ============================================================================

interface TestTreeProps {
  useCases: RunTestTreeResponse["use_cases"];
  expandedIds: Set<string>;
  onToggle: (id: string) => void;
  onTestClick?: (test: TestResult) => void;
  onRerunTest?: (testId: string) => void;
  filter?: string;
  suiteId?: number | null;
}

function getStatusIcon(status: string) {
  switch (status) {
    case "passed":
      return <CheckCircle className="h-4 w-4 text-success" />;
    case "failed":
    case "crashed":
      return <XCircle className="h-4 w-4 text-destructive" />;
    case "running":
      return <Loader2 className="h-4 w-4 text-primary animate-spin" />;
    case "skipped":
      return <AlertCircle className="h-4 w-4 text-warning" />;
    default:
      return <Circle className="h-4 w-4 text-muted-foreground" />;
  }
}

function TestTree({ useCases, expandedIds, onToggle, onTestClick, onRerunTest, filter, suiteId }: TestTreeProps) {
  if (!useCases || useCases.length === 0) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center justify-center py-12 text-muted-foreground">
          <FolderOpen className="h-12 w-12 mb-4 opacity-50" />
          <p>No tests in this run</p>
        </CardContent>
      </Card>
    );
  }

  // Filter use cases and tests
  const filteredUseCases = useCases.map((uc) => ({
    ...uc,
    tests: filter
      ? uc.tests.filter((t) => t.status === filter)
      : uc.tests,
  })).filter((uc) => uc.tests.length > 0);

  return (
    <Card>
      <CardContent className="p-4">
        <div className="space-y-1">
          {filteredUseCases.map((uc) => {
            const isExpanded = expandedIds.has(uc.use_case);
            const totalInUc = uc.tests.length;
            const completedInUc = uc.passed + uc.failed;
            const hasRunning = uc.running > 0;
            const hasFailed = uc.failed > 0;

            return (
              <div key={uc.use_case} className="border rounded-md overflow-hidden">
                {/* Use Case Header */}
                <button
                  onClick={() => onToggle(uc.use_case)}
                  className={cn(
                    "flex items-center gap-2 w-full p-3 text-left hover:bg-muted/50 transition-colors",
                    hasRunning && "bg-primary/5",
                    hasFailed && "bg-destructive/5"
                  )}
                >
                  {isExpanded ? (
                    <ChevronDown className="h-4 w-4 text-muted-foreground" />
                  ) : (
                    <ChevronRight className="h-4 w-4 text-muted-foreground" />
                  )}
                  {isExpanded ? (
                    <FolderOpen className="h-4 w-4 text-amber-500" />
                  ) : (
                    <Folder className="h-4 w-4 text-amber-500" />
                  )}
                  <span className="font-medium flex-1">{uc.use_case}</span>

                  {/* Progress indicator */}
                  <div className="flex items-center gap-2 text-xs">
                    {uc.running > 0 && (
                      <Badge variant="outline" className="text-primary border-primary">
                        {uc.running} running
                      </Badge>
                    )}
                    <span className="text-muted-foreground">
                      {completedInUc}/{totalInUc}
                    </span>
                    {uc.passed > 0 && (
                      <span className="text-success">{uc.passed}✓</span>
                    )}
                    {uc.failed > 0 && (
                      <span className="text-destructive">{uc.failed}✗</span>
                    )}
                  </div>
                </button>

                {/* Tests */}
                {isExpanded && (
                  <div className="border-t bg-muted/20">
                    {uc.tests.map((test) => (
                      <div
                        key={test.test_id}
                        className={cn(
                          "flex items-center gap-3 px-4 py-2 pl-10 hover:bg-muted/30 transition-colors group",
                          test.status === "running" && "bg-primary/10",
                          (test.status === "failed" || test.status === "crashed") && "bg-destructive/5"
                        )}
                      >
                        <button
                          onClick={() => onTestClick?.(test)}
                          className={cn(
                            "flex items-center gap-3 flex-1 min-w-0 text-left",
                            onTestClick && "cursor-pointer"
                          )}
                        >
                          {getStatusIcon(test.status)}
                          <div className="flex-1 min-w-0">
                            <p className="text-sm truncate">
                              {test.name || test.test_case}
                            </p>
                            {test.error_message && (
                              <p className="text-xs text-destructive truncate mt-1">
                                {test.error_message}
                              </p>
                            )}
                          </div>
                        </button>
                        <div className="flex items-center gap-2 text-xs text-muted-foreground">
                          {test.duration_ms !== null && (
                            <span className="font-mono">
                              {formatDuration(test.duration_ms)}
                            </span>
                          )}
                          {test.status === "running" && (
                            <span className="text-primary">(running)</span>
                          )}
                          {test.status === "pending" && (
                            <span>(pending)</span>
                          )}
                          {suiteId && onRerunTest && (
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                onRerunTest(test.test_id);
                              }}
                              className="p-1 rounded hover:bg-primary/20 opacity-0 group-hover:opacity-100 transition-opacity"
                              title="Rerun this test"
                            >
                              <Play className="h-3.5 w-3.5 text-primary" />
                            </button>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
}

// ============================================================================
// TestDetailDialog Component
// ============================================================================

interface TestDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  testDetail: TestDetail | null;
  loading: boolean;
  error?: string | null;
  suiteId?: number | null;
  onRerunTest?: (testId: string) => void;
}

function TestDetailDialog({ open, onOpenChange, testDetail, loading, error, suiteId, onRerunTest }: TestDetailDialogProps) {
  const [expandedAssertions, setExpandedAssertions] = useState<Set<number>>(new Set());
  const [rerunning, setRerunning] = useState(false);

  const handleRerun = async () => {
    if (!testDetail || !suiteId || !onRerunTest) return;
    setRerunning(true);
    try {
      await onRerunTest(testDetail.test_id);
      // Close dialog so user can see the new test in live view
      onOpenChange(false);
    } finally {
      setRerunning(false);
    }
  };

  const toggleAssertion = (idx: number) => {
    setExpandedAssertions(prev => {
      const next = new Set(prev);
      if (next.has(idx)) {
        next.delete(idx);
      } else {
        next.add(idx);
      }
      return next;
    });
  };

  if (!testDetail && !loading && !error) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl max-h-[85vh] flex flex-col">
        <DialogHeader className="flex-shrink-0">
          <div className="flex items-center justify-between">
            <DialogTitle className="flex items-center gap-2">
              {testDetail && getStatusIcon(testDetail.status)}
              {error && <XCircle className="h-4 w-4 text-destructive" />}
              <span className="truncate">{testDetail?.name || testDetail?.test_id || "Test Details"}</span>
            </DialogTitle>
            {suiteId && testDetail && onRerunTest && (
              <Button
                size="sm"
                variant="outline"
                onClick={handleRerun}
                disabled={rerunning}
                className="ml-4"
              >
                {rerunning ? (
                  <Loader2 className="h-4 w-4 animate-spin mr-1" />
                ) : (
                  <Play className="h-4 w-4 mr-1" />
                )}
                Rerun
              </Button>
            )}
          </div>
        </DialogHeader>

        {loading ? (
          <div className="flex items-center justify-center py-12">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <XCircle className="h-12 w-12 text-destructive mb-4" />
            <p className="text-lg font-medium text-destructive mb-2">Failed to load test details</p>
            <p className="text-sm text-muted-foreground">{error}</p>
          </div>
        ) : testDetail ? (
          <ScrollArea className="h-[calc(85vh-120px)]">
            <div className="space-y-6 pr-4">
              {/* Test Info */}
              <div className="flex flex-wrap gap-4 text-sm">
                <div>
                  <span className="text-muted-foreground">Test ID: </span>
                  <span className="font-mono">{testDetail.test_id}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">Duration: </span>
                  <span className="font-mono">{formatDuration(testDetail.duration_ms)}</span>
                </div>
                <Badge variant={testDetail.status === "passed" ? "default" : "destructive"}>
                  {testDetail.status}
                </Badge>
              </div>

              {/* Error Message */}
              {testDetail.error_message && (
                <div className="rounded-md bg-destructive/10 border border-destructive/20 p-4">
                  <p className="text-sm font-medium text-destructive mb-2">Error</p>
                  <pre className="text-xs font-mono whitespace-pre-wrap text-destructive/90">
                    {testDetail.error_message}
                  </pre>
                </div>
              )}

              {/* Steps */}
              {testDetail.steps && testDetail.steps.length > 0 && (
                <div>
                  <h4 className="font-medium mb-3 flex items-center gap-2">
                    <Terminal className="h-4 w-4" />
                    Steps ({testDetail.steps.length})
                  </h4>
                  <div className="space-y-3">
                    {testDetail.steps.map((step, idx) => (
                      <div
                        key={step.id || idx}
                        className={cn(
                          "rounded-md border p-3",
                          step.status === "passed" && "border-success/30 bg-success/5",
                          (step.status === "failed" || step.status === "crashed") && "border-destructive/30 bg-destructive/5"
                        )}
                      >
                        <div className="flex items-center justify-between mb-2">
                          <div className="flex items-center gap-2">
                            {step.status === "passed" ? (
                              <CheckCircle className="h-4 w-4 text-success" />
                            ) : step.status === "failed" || step.status === "crashed" ? (
                              <XCircle className="h-4 w-4 text-destructive" />
                            ) : (
                              <Circle className="h-4 w-4 text-muted-foreground" />
                            )}
                            <span className="text-sm font-medium">
                              Step {step.step_index + 1}: {step.description || step.phase}
                            </span>
                            {step.handler && (
                              <Badge variant="outline" className="text-xs">
                                {step.handler}
                              </Badge>
                            )}
                          </div>
                          {step.duration_ms !== null && (
                            <span className="text-xs font-mono text-muted-foreground">
                              {formatDuration(step.duration_ms)}
                            </span>
                          )}
                        </div>

                        {/* Step Error */}
                        {step.error_message && (
                          <div className="mt-2 p-2 rounded bg-destructive/10 text-xs font-mono text-destructive">
                            {step.error_message}
                          </div>
                        )}

                        {/* Stdout */}
                        {step.stdout && (
                          <div className="mt-2">
                            <p className="text-xs text-muted-foreground mb-1">stdout:</p>
                            <pre className="p-2 rounded bg-muted text-xs font-mono overflow-x-auto whitespace-pre-wrap max-h-40">
                              {step.stdout}
                            </pre>
                          </div>
                        )}

                        {/* Stderr */}
                        {step.stderr && (
                          <div className="mt-2">
                            <p className="text-xs text-muted-foreground mb-1">stderr:</p>
                            <pre className="p-2 rounded bg-destructive/10 text-xs font-mono overflow-x-auto whitespace-pre-wrap max-h-40 text-destructive/90">
                              {step.stderr}
                            </pre>
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Assertions */}
              {testDetail.assertions && testDetail.assertions.length > 0 && (
                <div>
                  <h4 className="font-medium mb-3 flex items-center gap-2">
                    <FileText className="h-4 w-4" />
                    Assertions ({testDetail.assertions.length})
                  </h4>
                  <div className="space-y-2">
                    {testDetail.assertions.map((assertion, idx) => {
                      const isExpanded = expandedAssertions.has(idx);
                      const hasDetails = assertion.actual_value || assertion.expected_value;

                      return (
                        <div
                          key={assertion.id || idx}
                          className={cn(
                            "rounded-md border",
                            assertion.passed
                              ? "border-success/30 bg-success/5"
                              : "border-destructive/30 bg-destructive/5"
                          )}
                        >
                          <div
                            className={cn(
                              "flex items-center gap-2 p-3",
                              hasDetails && "cursor-pointer hover:bg-muted/30"
                            )}
                            onClick={() => hasDetails && toggleAssertion(idx)}
                          >
                            {hasDetails && (
                              isExpanded ? (
                                <ChevronDown className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                              ) : (
                                <ChevronRight className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                              )
                            )}
                            {assertion.passed ? (
                              <CheckCircle className="h-4 w-4 text-success flex-shrink-0" />
                            ) : (
                              <XCircle className="h-4 w-4 text-destructive flex-shrink-0" />
                            )}
                            <span className="text-sm">
                              {assertion.message || assertion.expression}
                            </span>
                          </div>
                          {isExpanded && hasDetails && (
                            <div className="px-3 pb-3 pt-0 border-t border-border/50">
                              <div className="mt-2 text-xs font-mono space-y-1">
                                <p className="text-muted-foreground">
                                  <span className="text-foreground/70">Expression:</span>{" "}
                                  <code className="bg-muted px-1 py-0.5 rounded">{assertion.expression}</code>
                                </p>
                                {assertion.expected_value && (
                                  <p className="text-muted-foreground">
                                    <span className="text-foreground/70">Expected:</span>{" "}
                                    <code className="bg-muted px-1 py-0.5 rounded">{assertion.expected_value}</code>
                                  </p>
                                )}
                                {assertion.actual_value && (
                                  <p className="text-muted-foreground">
                                    <span className="text-foreground/70">Actual:</span>{" "}
                                    <code className={cn(
                                      "px-1 py-0.5 rounded",
                                      assertion.passed ? "bg-success/20" : "bg-destructive/20"
                                    )}>{assertion.actual_value}</code>
                                  </p>
                                )}
                              </div>
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}

              {/* No details */}
              {(!testDetail.steps || testDetail.steps.length === 0) && (!testDetail.assertions || testDetail.assertions.length === 0) && (
                <div className="text-center py-8 text-muted-foreground">
                  <Terminal className="h-12 w-12 mx-auto mb-4 opacity-50" />
                  <p>No step or assertion details available for this test.</p>
                </div>
              )}
            </div>
          </ScrollArea>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

// ============================================================================
// Main LiveFeed Component
// ============================================================================

export function LiveFeed() {
  const router = useRouter();
  const { events, connected, currentRunId } = useLiveEvents({ maxEvents: 500 });
  const [run, setRun] = useState<RunExtended | null>(null);
  const [testTree, setTestTree] = useState<RunTestTreeResponse | null>(null);
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [statusFilter, setStatusFilter] = useState<string>("");
  // Track the displayed run ID separately - persists after run completes
  const [displayedRunId, setDisplayedRunId] = useState<string | null>(null);
  // Test detail dialog state
  const [selectedTest, setSelectedTest] = useState<TestResult | null>(null);
  const [testDetail, setTestDetail] = useState<TestDetail | null>(null);
  const [testDetailLoading, setTestDetailLoading] = useState(false);
  const [testDetailError, setTestDetailError] = useState<string | null>(null);
  // Cancel and rerun state
  const [cancelling, setCancelling] = useState(false);
  const [rerunning, setRerunning] = useState(false);

  // Update displayed run ID when a new run starts (but don't clear on completion)
  useEffect(() => {
    if (currentRunId && currentRunId !== displayedRunId) {
      // New run started - switch to it
      setDisplayedRunId(currentRunId);
      setExpandedIds(new Set()); // Reset expanded state for new run
    }
  }, [currentRunId, displayedRunId]);

  // Fetch run data when we have a displayed run_id
  useEffect(() => {
    if (!displayedRunId) {
      setRun(null);
      setTestTree(null);
      return;
    }

    const fetchData = async () => {
      try {
        const [runData, treeData] = await Promise.all([
          getRunExtended(displayedRunId),
          getRunTestsTree(displayedRunId),
        ]);
        setRun(runData);
        setTestTree(treeData);

        // Auto-expand use cases with running tests
        const ucWithRunning = treeData.use_cases
          .filter((uc) => uc.running > 0)
          .map((uc) => uc.use_case);
        if (ucWithRunning.length > 0) {
          setExpandedIds((prev) => new Set([...prev, ...ucWithRunning]));
        }
      } catch (err) {
        console.error("Failed to fetch run data:", err);
      }
    };

    fetchData();
  }, [displayedRunId]);

  // Refresh data on SSE events
  useEffect(() => {
    if (!displayedRunId || events.length === 0) return;

    const latestEvent = events[0];
    if (
      latestEvent.type === "test_started" ||
      latestEvent.type === "test_completed" ||
      latestEvent.type === "run_completed" ||
      latestEvent.type === "run_cancelled"
    ) {
      // Refetch data
      Promise.all([
        getRunExtended(displayedRunId),
        getRunTestsTree(displayedRunId),
      ]).then(([runData, treeData]) => {
        setRun(runData);
        setTestTree(treeData);
      }).catch(console.error);
    }
  }, [events, displayedRunId]);

  // Track elapsed time for running tests and run duration (force re-render every 100ms)
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!displayedRunId) return;

    // Check if we need to update: run is in progress OR there are running tests
    const runInProgress = run?.status === "running" || run?.status === "pending";
    const runningTests = testTree?.use_cases
      .flatMap((uc) => uc.tests)
      .filter((t) => t.status === "running") || [];

    if (!runInProgress && runningTests.length === 0) {
      return;
    }

    // Force re-render every 100ms to update elapsed times
    const interval = setInterval(() => {
      setTick((t) => t + 1);
    }, 100);

    return () => clearInterval(interval);
  }, [displayedRunId, testTree, run?.status]);

  const toggleExpanded = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  // Handle test click - fetch test detail
  const handleTestClick = useCallback(async (test: TestResult) => {
    if (!displayedRunId) return;

    setSelectedTest(test);
    setTestDetail(null);
    setTestDetailError(null);
    setTestDetailLoading(true);

    try {
      const detail = await getTestDetail(displayedRunId, test.id);
      setTestDetail(detail);
    } catch (err) {
      console.error("Failed to fetch test detail:", err);
      setTestDetailError(err instanceof Error ? err.message : "Failed to load test details");
    } finally {
      setTestDetailLoading(false);
    }
  }, [displayedRunId]);

  // Handle cancel
  const handleCancel = useCallback(async () => {
    if (!displayedRunId) return;

    setCancelling(true);
    try {
      await cancelRun(displayedRunId);
      // Refetch run data to update status
      const runData = await getRunExtended(displayedRunId);
      setRun(runData);
    } catch (err) {
      console.error("Failed to cancel run:", err);
    } finally {
      setCancelling(false);
    }
  }, [displayedRunId]);

  // Handle rerun - reruns the same tests from this run
  const handleRerun = useCallback(async () => {
    if (!run) return;

    setRerunning(true);
    try {
      await rerunFromRun(run);
      // New run will be picked up by SSE events automatically
    } catch (err) {
      console.error("Failed to rerun:", err);
    } finally {
      setRerunning(false);
    }
  }, [run]);

  // Handle rerun test
  const handleRerunTest = useCallback(async (testId: string) => {
    if (!run?.suite_id) return;
    try {
      await runTests(run.suite_id, { tc: testId });
      // New run will be picked up by SSE events automatically
      router.push("/live");
    } catch (err) {
      console.error("Failed to rerun test:", err);
    }
  }, [run?.suite_id, router]);

  // Calculate stats
  const stats = useMemo(() => {
    if (!run) {
      return { pending: 0, running: 0, passed: 0, failed: 0 };
    }
    return {
      pending: run.pending_count,
      running: run.running_count,
      passed: run.passed,
      failed: run.failed,
    };
  }, [run]);

  // Find all currently running tests
  const runningTests = useMemo(() => {
    if (!testTree) return [];
    return testTree.use_cases
      .flatMap((uc) => uc.tests)
      .filter((t) => t.status === "running");
  }, [testTree]);

  // Calculate elapsed time for a test
  const getTestElapsed = useCallback((test: TestResult) => {
    const startTime = test.started_at
      ? new Date(test.started_at).getTime()
      : Date.now();
    return Date.now() - startTime;
  }, []);

  // Calculate elapsed time for the entire run
  // Note: No useMemo - we want this to recalculate on every tick (100ms interval)
  const runElapsed = (() => {
    if (!run?.started_at) return 0;
    const startTime = new Date(run.started_at).getTime();
    if (run.finished_at) {
      // Run completed - use final duration
      return new Date(run.finished_at).getTime() - startTime;
    }
    // Run in progress - calculate from now
    return Date.now() - startTime;
  })();

  return (
    <div className="space-y-6">
      {/* Connection Status */}
      <Card className="border-border bg-card rounded-md">
        <CardContent className="flex items-center justify-between p-4">
          <div className="flex items-center gap-3">
            {connected ? (
              <>
                <div className="flex h-10 w-10 items-center justify-center rounded bg-success/20">
                  <Wifi className="h-5 w-5 text-success" />
                </div>
                <div>
                  <p className="font-medium text-foreground">Connected</p>
                  <p className="text-sm text-muted-foreground">
                    Receiving live events
                  </p>
                </div>
              </>
            ) : (
              <>
                <div className="flex h-10 w-10 items-center justify-center rounded bg-destructive/20">
                  <WifiOff className="h-5 w-5 text-destructive" />
                </div>
                <div>
                  <p className="font-medium text-foreground">Disconnected</p>
                  <p className="text-sm text-muted-foreground">
                    Attempting to reconnect...
                  </p>
                </div>
              </>
            )}
          </div>

          {displayedRunId && (
            <div className="flex items-center gap-2 rounded bg-primary/10 px-4 py-2">
              {currentRunId === displayedRunId && run?.status === "running" && !run?.cancel_requested ? (
                <span className="relative flex h-3 w-3">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75"></span>
                  <span className="relative inline-flex h-3 w-3 rounded-full bg-primary"></span>
                </span>
              ) : (
                <span className="flex h-3 w-3 rounded-full bg-muted-foreground"></span>
              )}
              <span className="font-mono text-sm font-medium text-primary">
                Run: {displayedRunId.slice(0, 8)}...
              </span>
              {run?.status === "cancelled" && (
                <span className="text-xs text-warning">(cancelled)</span>
              )}
              {run?.cancel_requested && run?.status === "running" && (
                <span className="text-xs text-warning">(cancelling...)</span>
              )}
              {currentRunId !== displayedRunId && run?.status === "completed" && (
                <span className="text-xs text-muted-foreground">(completed)</span>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      {displayedRunId && run ? (
        <>
          {/* Stats Cards */}
          <StatsCards
            {...stats}
            onFilterClick={setStatusFilter}
            activeFilter={statusFilter || null}
          />

          {/* Progress Bar */}
          <ProgressBarSection
            completed={stats.passed + stats.failed}
            total={run.total_tests}
            passed={stats.passed}
            failed={stats.failed}
          />

          {/* Currently Running - show as long as there are running tests */}
          {(run.status === "running" || run.status === "pending") && runningTests.length > 0 && (
            <CurrentlyRunning tests={runningTests} getElapsed={getTestElapsed} />
          )}

          {/* Run Info */}
          <Card>
            <CardContent className="p-4">
              <div className="flex items-center justify-between text-sm">
                <div className="flex items-center gap-4">
                  <span className="text-muted-foreground">
                    Run: <span className="font-mono">{displayedRunId.slice(0, 12)}</span>
                  </span>
                  <Badge variant="outline">
                    {run.cancel_requested && run.status === "running"
                      ? "cancelling"
                      : run.status}
                  </Badge>
                  <Badge variant="outline">{run.mode}</Badge>
                  <span className="text-muted-foreground">
                    {run.total_tests} tests
                  </span>
                </div>
                <div className="flex items-center gap-4">
                  {run.started_at && (
                    <span className="text-muted-foreground">
                      Started: {new Date(run.started_at).toLocaleTimeString()}
                    </span>
                  )}
                  {run.started_at && (
                    <span className="text-muted-foreground font-mono">
                      Elapsed: {formatDuration(runElapsed)}
                    </span>
                  )}
                  {/* Cancel button - show for running/pending runs */}
                  {(run.status === "running" || run.status === "pending") && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleCancel}
                      disabled={cancelling || run.cancel_requested}
                      className="gap-2 text-destructive border-destructive/50 hover:bg-destructive/10"
                    >
                      {cancelling ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <StopCircle className="h-4 w-4" />
                      )}
                      {run.cancel_requested ? "Cancelling..." : "Cancel"}
                    </Button>
                  )}
                  {/* Rerun button - show for completed/cancelled/failed runs */}
                  {run.suite_id && (run.status === "completed" || run.status === "cancelled" || run.status === "failed") && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleRerun}
                      disabled={rerunning}
                      className="gap-2"
                    >
                      {rerunning ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <RotateCcw className="h-4 w-4" />
                      )}
                      Rerun
                    </Button>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Test Tree */}
          {testTree && (
            <TestTree
              useCases={testTree.use_cases}
              expandedIds={expandedIds}
              onToggle={toggleExpanded}
              onTestClick={handleTestClick}
              onRerunTest={handleRerunTest}
              filter={statusFilter}
              suiteId={run.suite_id}
            />
          )}
        </>
      ) : (
        /* No Active Run */
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-16 text-center">
            <Clock className="h-16 w-16 text-muted-foreground/30 mb-4" />
            <h3 className="text-lg font-medium mb-2">No Active Run</h3>
            <p className="text-sm text-muted-foreground max-w-md">
              Start a test run from the Tests page or CLI to see live progress here.
              The dashboard will automatically update when a new run begins.
            </p>
          </CardContent>
        </Card>
      )}

      {/* Test Detail Dialog */}
      <TestDetailDialog
        open={!!selectedTest}
        onOpenChange={(open) => {
          if (!open) {
            setSelectedTest(null);
            setTestDetail(null);
            setTestDetailError(null);
          }
        }}
        testDetail={testDetail}
        loading={testDetailLoading}
        error={testDetailError}
        suiteId={run?.suite_id}
        onRerunTest={handleRerunTest}
      />
    </div>
  );
}
