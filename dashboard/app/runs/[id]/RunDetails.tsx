"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  RunSummary,
  TestResult,
  TestDetail,
  formatDuration,
  formatRelativeTime,
  getStatusBgColor,
  getTestDetail,
  rerunFromRun,
  cancelRun,
} from "@/lib/api";
import {
  CheckCircle,
  XCircle,
  Clock,
  ChevronDown,
  ChevronRight,
  Terminal,
  AlertCircle,
  Loader2,
  Circle,
  FolderOpen,
  Folder,
  FileText,
  RotateCcw,
  StopCircle,
} from "lucide-react";
import { cn } from "@/lib/utils";

interface RunDetailsProps {
  run: RunSummary;
  tests: TestResult[];
}

// Group tests by use_case
interface UseCaseGroup {
  use_case: string;
  tests: TestResult[];
  passed: number;
  failed: number;
  pending: number;
  running: number;
}

function groupTestsByUseCase(tests: TestResult[]): UseCaseGroup[] {
  const groups = new Map<string, TestResult[]>();

  for (const test of tests) {
    const uc = test.use_case || "unknown";
    if (!groups.has(uc)) {
      groups.set(uc, []);
    }
    groups.get(uc)!.push(test);
  }

  return Array.from(groups.entries()).map(([use_case, tests]) => ({
    use_case,
    tests,
    passed: tests.filter(t => t.status === "passed").length,
    failed: tests.filter(t => t.status === "failed" || t.status === "crashed").length,
    crashed: tests.filter(t => t.status === "crashed").length,
    skipped: tests.filter(t => t.status === "skipped").length,
    pending: tests.filter(t => t.status === "pending").length,
    running: tests.filter(t => t.status === "running").length,
    total: tests.length,
  }));
}

export function RunDetails({ run, tests }: RunDetailsProps) {
  const router = useRouter();
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [filter, setFilter] = useState<string | null>(null);
  const [selectedTest, setSelectedTest] = useState<TestResult | null>(null);
  const [testDetail, setTestDetail] = useState<TestDetail | null>(null);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [rerunning, setRerunning] = useState(false);
  const [cancelling, setCancelling] = useState(false);

  const useCases = useMemo(() => groupTestsByUseCase(tests), [tests]);

  const handleRerun = async () => {
    if (!run.suite_id) return;

    setRerunning(true);
    try {
      await rerunFromRun(run);
      router.push("/live");
    } catch (error) {
      console.error("Failed to rerun:", error);
    } finally {
      setRerunning(false);
    }
  };

  const handleCancel = async () => {
    setCancelling(true);
    try {
      await cancelRun(run.run_id);
      router.refresh();
    } catch (error) {
      console.error("Failed to cancel:", error);
    } finally {
      setCancelling(false);
    }
  };

  const canCancel = run.status === "pending" || run.status === "running";
  const isCancelRequested = run.cancel_requested;

  const toggleExpand = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const handleTestClick = async (test: TestResult) => {
    setSelectedTest(test);
    setLoadingDetail(true);
    try {
      const detail = await getTestDetail(run.run_id, test.id);
      setTestDetail(detail);
    } catch (error) {
      console.error("Failed to load test detail:", error);
    } finally {
      setLoadingDetail(false);
    }
  };

  const handleDialogClose = () => {
    setSelectedTest(null);
    setTestDetail(null);
  };

  const stats = {
    passed: tests.filter(t => t.status === "passed").length,
    failed: tests.filter(t => t.status === "failed" || t.status === "crashed").length,
  };

  return (
    <div className="space-y-6">
      {/* Run Summary Card */}
      <Card className="border-border bg-card rounded-md">
        <CardContent className="p-6">
          {/* Header with Rerun/Cancel buttons */}
          <div className="flex items-center justify-between mb-6">
            <h3 className="text-lg font-medium">Run Summary</h3>
            <div className="flex items-center gap-2">
              {/* Cancel button - show for running/pending runs */}
              {canCancel && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleCancel}
                  disabled={cancelling || isCancelRequested}
                  className="gap-2 text-destructive border-destructive/50 hover:bg-destructive/10"
                >
                  {cancelling ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <StopCircle className="h-4 w-4" />
                  )}
                  {isCancelRequested ? "Cancelling..." : "Cancel"}
                </Button>
              )}
              {/* Rerun button */}
              {run.suite_id && (
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

          <div className="grid gap-6 md:grid-cols-4">
            <div>
              <p className="text-sm text-muted-foreground">Status</p>
              <Badge
                variant="secondary"
                className={`mt-1 ${getStatusBgColor(
                  isCancelRequested && run.status === "running"
                    ? "cancelled"
                    : run.status
                )}`}
              >
                {isCancelRequested && run.status === "running"
                  ? "cancelling"
                  : run.status}
              </Badge>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Duration</p>
              <p className="mt-1 flex items-center gap-1 text-lg font-semibold">
                <Clock className="h-4 w-4 text-accent" />
                {formatDuration(run.duration_ms)}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Started</p>
              <p className="mt-1 text-lg font-semibold">
                {formatRelativeTime(run.started_at)}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Tests</p>
              <div className="mt-1 flex items-center gap-2">
                <span className="flex items-center gap-1 text-lg font-semibold text-success">
                  <CheckCircle className="h-4 w-4" />
                  {run.passed}
                </span>
                <span className="text-muted-foreground">/</span>
                <span className="flex items-center gap-1 text-lg font-semibold text-destructive">
                  <XCircle className="h-4 w-4" />
                  {run.failed}
                </span>
              </div>
            </div>
          </div>

          {/* Metadata */}
          {(run.cli_version || run.docker_image) && (
            <div className="mt-6 flex gap-4 border-t border-border pt-4">
              {run.cli_version && (
                <div>
                  <p className="text-xs text-muted-foreground">CLI Version</p>
                  <p className="font-mono text-sm">{run.cli_version}</p>
                </div>
              )}
              {run.docker_image && (
                <div>
                  <p className="text-xs text-muted-foreground">Docker Image</p>
                  <p className="font-mono text-sm">{run.docker_image}</p>
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Stats Cards */}
      <div className="grid gap-4 md:grid-cols-2">
        <Card
          className={cn(
            "cursor-pointer transition-all border-2",
            filter === "passed"
              ? "border-success bg-success/10"
              : "border-transparent hover:border-success/50"
          )}
          onClick={() => setFilter(filter === "passed" ? null : "passed")}
        >
          <CardContent className="flex items-center gap-4 p-4">
            <CheckCircle className="h-8 w-8 text-success" />
            <div>
              <p className="text-2xl font-bold">{stats.passed}</p>
              <p className="text-sm text-muted-foreground">Passed</p>
            </div>
          </CardContent>
        </Card>
        <Card
          className={cn(
            "cursor-pointer transition-all border-2",
            filter === "failed"
              ? "border-destructive bg-destructive/10"
              : "border-transparent hover:border-destructive/50"
          )}
          onClick={() => setFilter(filter === "failed" ? null : "failed")}
        >
          <CardContent className="flex items-center gap-4 p-4">
            <XCircle className="h-8 w-8 text-destructive" />
            <div>
              <p className="text-2xl font-bold">{stats.failed}</p>
              <p className="text-sm text-muted-foreground">Failed</p>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Test Tree */}
      <TestTree
        useCases={useCases}
        expandedIds={expandedIds}
        onToggle={toggleExpand}
        onTestClick={handleTestClick}
        filter={filter}
      />

      {/* Test Detail Dialog */}
      <TestDetailDialog
        open={selectedTest !== null}
        onOpenChange={(open) => !open && handleDialogClose()}
        testDetail={testDetail}
        loading={loadingDetail}
      />
    </div>
  );
}

// ============================================================================
// TestTree Component
// ============================================================================

interface TestTreeProps {
  useCases: UseCaseGroup[];
  expandedIds: Set<string>;
  onToggle: (id: string) => void;
  onTestClick?: (test: TestResult) => void;
  filter?: string | null;
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

function TestTree({ useCases, expandedIds, onToggle, onTestClick, filter }: TestTreeProps) {
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
  // When filtering by "failed", include "crashed" tests as well
  const filteredUseCases = useCases.map((uc) => ({
    ...uc,
    tests: filter
      ? uc.tests.filter((t) =>
          filter === "failed"
            ? t.status === "failed" || t.status === "crashed"
            : t.status === filter
        )
      : uc.tests,
  })).filter((uc) => uc.tests.length > 0);

  if (filteredUseCases.length === 0) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center justify-center py-12 text-muted-foreground">
          <FolderOpen className="h-12 w-12 mb-4 opacity-50" />
          <p>No {filter} tests</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="p-4">
        <div className="space-y-1">
          {filteredUseCases.map((uc) => {
            const isExpanded = expandedIds.has(uc.use_case);
            const totalInUc = uc.tests.length;
            const hasFailed = uc.failed > 0;

            return (
              <div key={uc.use_case} className="border rounded-md overflow-hidden">
                {/* Use Case Header */}
                <button
                  onClick={() => onToggle(uc.use_case)}
                  className={cn(
                    "flex items-center gap-2 w-full p-3 text-left hover:bg-muted/50 transition-colors",
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
                    <span className="text-muted-foreground">
                      {totalInUc}
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
                      <button
                        key={test.id}
                        onClick={() => onTestClick?.(test)}
                        className={cn(
                          "flex items-center gap-3 px-4 py-2 pl-10 hover:bg-muted/30 transition-colors w-full text-left",
                          (test.status === "failed" || test.status === "crashed") && "bg-destructive/5",
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
                        <div className="flex items-center gap-2 text-xs text-muted-foreground">
                          {test.duration_ms !== null && (
                            <span className="font-mono">
                              {formatDuration(test.duration_ms)}
                            </span>
                          )}
                        </div>
                      </button>
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
}

function TestDetailDialog({ open, onOpenChange, testDetail, loading }: TestDetailDialogProps) {
  if (!testDetail && !loading) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl max-h-[85vh] flex flex-col">
        <DialogHeader className="flex-shrink-0">
          <DialogTitle className="flex items-center gap-2">
            {testDetail && getStatusIcon(testDetail.status)}
            <span className="truncate">{testDetail?.name || testDetail?.test_id}</span>
          </DialogTitle>
        </DialogHeader>

        {loading ? (
          <div className="flex items-center justify-center py-12">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </div>
        ) : testDetail ? (
          <div className="overflow-y-auto max-h-[calc(85vh-120px)] pr-2">
            <div className="space-y-6">
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
                    {testDetail.assertions.map((assertion, idx) => (
                      <div
                        key={assertion.id || idx}
                        className={cn(
                          "rounded-md border p-3",
                          assertion.passed
                            ? "border-success/30 bg-success/5"
                            : "border-destructive/30 bg-destructive/5"
                        )}
                      >
                        <div className="flex items-center gap-2">
                          {assertion.passed ? (
                            <CheckCircle className="h-4 w-4 text-success" />
                          ) : (
                            <XCircle className="h-4 w-4 text-destructive" />
                          )}
                          <span className="text-sm">
                            {assertion.message || assertion.expression}
                          </span>
                        </div>
                        {!assertion.passed && assertion.actual_value && (
                          <div className="mt-2 text-xs font-mono text-muted-foreground">
                            <p>Expected: {assertion.expected_value}</p>
                            <p>Actual: {assertion.actual_value}</p>
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
