"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  ChevronRight,
  ChevronDown,
  FolderOpen,
  Folder,
  FileText,
  Container,
  Terminal,
  TestTube,
  Play,
  Loader2,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { Suite, SuiteTest, getSuite, runTests } from "@/lib/api";

interface TestsBrowserProps {
  suites: Suite[];
}

interface UseCaseGroup {
  useCase: string;
  testCases: TestCaseGroup[];
}

interface TestCaseGroup {
  testCase: string;
  tests: SuiteTest[];
}

function groupTestsByHierarchy(tests: SuiteTest[]): UseCaseGroup[] {
  const useCaseMap = new Map<string, Map<string, SuiteTest[]>>();

  for (const test of tests) {
    if (!useCaseMap.has(test.use_case)) {
      useCaseMap.set(test.use_case, new Map());
    }
    const testCaseMap = useCaseMap.get(test.use_case)!;
    if (!testCaseMap.has(test.test_case)) {
      testCaseMap.set(test.test_case, []);
    }
    testCaseMap.get(test.test_case)!.push(test);
  }

  return Array.from(useCaseMap.entries()).map(([useCase, testCaseMap]) => ({
    useCase,
    testCases: Array.from(testCaseMap.entries()).map(([testCase, tests]) => ({
      testCase,
      tests,
    })),
  }));
}

export function TestsBrowser({ suites }: TestsBrowserProps) {
  const router = useRouter();
  const [expandedSuites, setExpandedSuites] = useState<Set<number>>(new Set());
  const [expandedUseCases, setExpandedUseCases] = useState<Set<string>>(
    new Set()
  );
  const [suiteTests, setSuiteTests] = useState<Map<number, SuiteTest[]>>(
    new Map()
  );
  const [loadingSuites, setLoadingSuites] = useState<Set<number>>(new Set());

  // Running state for different levels
  const [runningSuite, setRunningSuite] = useState<number | null>(null);
  const [runningUc, setRunningUc] = useState<string | null>(null);
  const [runningTc, setRunningTc] = useState<string | null>(null);
  const [runMessage, setRunMessage] = useState<string | null>(null);

  const toggleSuite = async (suiteId: number) => {
    const newExpanded = new Set(expandedSuites);
    if (newExpanded.has(suiteId)) {
      newExpanded.delete(suiteId);
    } else {
      newExpanded.add(suiteId);
      // Load tests if not already loaded
      if (!suiteTests.has(suiteId)) {
        setLoadingSuites((prev) => new Set(prev).add(suiteId));
        try {
          const suiteData = await getSuite(suiteId);
          setSuiteTests((prev) => new Map(prev).set(suiteId, suiteData.tests));
        } catch (err) {
          console.error("Failed to load suite tests:", err);
        } finally {
          setLoadingSuites((prev) => {
            const next = new Set(prev);
            next.delete(suiteId);
            return next;
          });
        }
      }
    }
    setExpandedSuites(newExpanded);
  };

  const toggleUseCase = (key: string) => {
    const newExpanded = new Set(expandedUseCases);
    if (newExpanded.has(key)) {
      newExpanded.delete(key);
    } else {
      newExpanded.add(key);
    }
    setExpandedUseCases(newExpanded);
  };

  // Run handlers
  const handleRunSuite = async (
    e: React.MouseEvent,
    suiteId: number,
    suiteName: string
  ) => {
    e.stopPropagation();
    setRunningSuite(suiteId);
    setRunMessage(null);
    try {
      await runTests(suiteId);
      router.push("/live");
    } catch (err) {
      setRunMessage(
        `Error: ${err instanceof Error ? err.message : "Failed to start"}`
      );
      setTimeout(() => setRunMessage(null), 5000);
    } finally {
      setRunningSuite(null);
    }
  };

  const handleRunUc = async (
    e: React.MouseEvent,
    suiteId: number,
    uc: string
  ) => {
    e.stopPropagation();
    const key = `${suiteId}-${uc}`;
    setRunningUc(key);
    setRunMessage(null);
    try {
      await runTests(suiteId, { uc });
      router.push("/live");
    } catch (err) {
      setRunMessage(
        `Error: ${err instanceof Error ? err.message : "Failed to start"}`
      );
      setTimeout(() => setRunMessage(null), 5000);
    } finally {
      setRunningUc(null);
    }
  };

  const handleRunTc = async (
    e: React.MouseEvent,
    suiteId: number,
    tc: string
  ) => {
    e.stopPropagation();
    const key = `${suiteId}-${tc}`;
    setRunningTc(key);
    setRunMessage(null);
    try {
      await runTests(suiteId, { tc });
      router.push("/live");
    } catch (err) {
      setRunMessage(
        `Error: ${err instanceof Error ? err.message : "Failed to start"}`
      );
      setTimeout(() => setRunMessage(null), 5000);
    } finally {
      setRunningTc(null);
    }
  };

  if (suites.length === 0) {
    return (
      <Card className="rounded-md">
        <CardContent className="flex flex-col items-center justify-center py-12 text-center">
          <FolderOpen className="h-12 w-12 text-muted-foreground/50" />
          <h3 className="mt-4 text-lg font-medium">No test suites</h3>
          <p className="mt-2 text-sm text-muted-foreground">
            Add a test suite in Settings to browse tests
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      {/* Status message */}
      {runMessage && (
        <div
          className={cn(
            "p-3 rounded-md text-sm",
            runMessage.startsWith("Error")
              ? "bg-destructive/20 text-destructive"
              : "bg-green-500/20 text-green-500"
          )}
        >
          {runMessage}
        </div>
      )}

      <Card className="rounded-md">
        <CardHeader className="pb-4">
          <CardTitle className="text-lg font-medium flex items-center gap-2">
            <TestTube className="h-5 w-5" />
            Test Browser
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-1">
            {suites.map((suite) => {
              const isExpanded = expandedSuites.has(suite.id);
              const isLoading = loadingSuites.has(suite.id);
              const tests = suiteTests.get(suite.id) || [];
              const hierarchy = groupTestsByHierarchy(tests);
              const isSuiteRunning = runningSuite === suite.id;

              return (
                <div key={suite.id} className="border rounded-md overflow-hidden">
                  {/* Suite Header */}
                  <div
                    className="flex items-center gap-2 p-3 hover:bg-muted/50 transition-colors cursor-pointer"
                    onClick={() => toggleSuite(suite.id)}
                  >
                    <div className="flex items-center gap-2 flex-1">
                      {isExpanded ? (
                        <ChevronDown className="h-4 w-4 text-muted-foreground" />
                      ) : (
                        <ChevronRight className="h-4 w-4 text-muted-foreground" />
                      )}
                      {isExpanded ? (
                        <FolderOpen className="h-4 w-4 text-primary" />
                      ) : (
                        <Folder className="h-4 w-4 text-primary" />
                      )}
                      <span className="font-medium flex-1">
                        {suite.suite_name}
                      </span>
                    </div>
                    <Badge
                      variant="outline"
                      className={cn(
                        "text-xs",
                        suite.mode === "docker"
                          ? "border-blue-500/50 text-blue-500"
                          : "border-orange-500/50 text-orange-500"
                      )}
                    >
                      {suite.mode === "docker" ? (
                        <Container className="h-3 w-3 mr-1" />
                      ) : (
                        <Terminal className="h-3 w-3 mr-1" />
                      )}
                      {suite.mode}
                    </Badge>
                    <span className="text-xs text-muted-foreground">
                      {suite.test_count} tests
                    </span>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 text-green-500 hover:text-green-600 hover:bg-green-500/10"
                      onClick={(e) =>
                        handleRunSuite(e, suite.id, suite.suite_name)
                      }
                      disabled={isSuiteRunning}
                      title="Run all tests in suite"
                    >
                      {isSuiteRunning ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Play className="h-4 w-4" />
                      )}
                    </Button>
                  </div>

                  {/* Suite Content */}
                  {isExpanded && (
                    <div className="border-t bg-muted/20">
                      {isLoading ? (
                        <div className="p-4 text-sm text-muted-foreground text-center">
                          Loading tests...
                        </div>
                      ) : hierarchy.length === 0 ? (
                        <div className="p-4 text-sm text-muted-foreground text-center">
                          No tests found
                        </div>
                      ) : (
                        <div className="py-1">
                          {hierarchy.map((ucGroup) => {
                            const ucKey = `${suite.id}-${ucGroup.useCase}`;
                            const isUcExpanded = expandedUseCases.has(ucKey);
                            const isUcRunning = runningUc === ucKey;

                            return (
                              <div key={ucKey}>
                                {/* Use Case Header */}
                                <div
                                  className="flex items-center gap-2 px-3 py-2 pl-8 hover:bg-muted/50 transition-colors cursor-pointer"
                                  onClick={() => toggleUseCase(ucKey)}
                                >
                                  <div className="flex items-center gap-2 flex-1">
                                    {isUcExpanded ? (
                                      <ChevronDown className="h-3 w-3 text-muted-foreground" />
                                    ) : (
                                      <ChevronRight className="h-3 w-3 text-muted-foreground" />
                                    )}
                                    {isUcExpanded ? (
                                      <FolderOpen className="h-4 w-4 text-amber-500" />
                                    ) : (
                                      <Folder className="h-4 w-4 text-amber-500" />
                                    )}
                                    <span className="text-sm font-medium">
                                      {ucGroup.useCase}
                                    </span>
                                  </div>
                                  <span className="text-xs text-muted-foreground">
                                    {ucGroup.testCases.reduce(
                                      (sum, tc) => sum + tc.tests.length,
                                      0
                                    )}{" "}
                                    tests
                                  </span>
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    className="h-6 w-6 text-green-500 hover:text-green-600 hover:bg-green-500/10"
                                    onClick={(e) =>
                                      handleRunUc(e, suite.id, ucGroup.useCase)
                                    }
                                    disabled={isUcRunning}
                                    title={`Run all tests in ${ucGroup.useCase}`}
                                  >
                                    {isUcRunning ? (
                                      <Loader2 className="h-3 w-3 animate-spin" />
                                    ) : (
                                      <Play className="h-3 w-3" />
                                    )}
                                  </Button>
                                </div>

                                {/* Test Cases */}
                                {isUcExpanded && (
                                  <div className="py-1">
                                    {ucGroup.testCases.map((tcGroup) => {
                                      const tcId = `${ucGroup.useCase}/${tcGroup.testCase}`;
                                      const tcKey = `${suite.id}-${tcId}`;
                                      const isTcRunning = runningTc === tcKey;

                                      return (
                                        <div
                                          key={tcGroup.testCase}
                                          className="pl-14"
                                        >
                                          {/* Test Case Header */}
                                          <div className="flex items-center gap-2 px-3 py-1.5 hover:bg-muted/30 transition-colors">
                                            <Folder className="h-3.5 w-3.5 text-cyan-500" />
                                            <span className="text-sm text-muted-foreground flex-1">
                                              {tcGroup.testCase}
                                            </span>
                                            <Button
                                              variant="ghost"
                                              size="icon"
                                              className="h-5 w-5 text-green-500 hover:text-green-600 hover:bg-green-500/10"
                                              onClick={(e) =>
                                                handleRunTc(e, suite.id, tcId)
                                              }
                                              disabled={isTcRunning}
                                              title={`Run ${tcId}`}
                                            >
                                              {isTcRunning ? (
                                                <Loader2 className="h-3 w-3 animate-spin" />
                                              ) : (
                                                <Play className="h-3 w-3" />
                                              )}
                                            </Button>
                                          </div>

                                          {/* Individual Tests */}
                                          {tcGroup.tests.map((test) => (
                                            <div
                                              key={test.test_id}
                                              className="flex items-center gap-2 px-3 py-1.5 pl-10 hover:bg-muted/30 transition-colors"
                                            >
                                              <FileText className="h-3.5 w-3.5 text-muted-foreground" />
                                              <span className="text-sm flex-1 truncate">
                                                {test.name || test.test_id}
                                              </span>
                                              {test.tags.length > 0 && (
                                                <div className="flex items-center gap-1">
                                                  {test.tags
                                                    .slice(0, 3)
                                                    .map((tag) => (
                                                      <Badge
                                                        key={tag}
                                                        variant="secondary"
                                                        className="text-[10px] px-1.5 py-0"
                                                      >
                                                        {tag}
                                                      </Badge>
                                                    ))}
                                                  {test.tags.length > 3 && (
                                                    <span className="text-[10px] text-muted-foreground">
                                                      +{test.tags.length - 3}
                                                    </span>
                                                  )}
                                                </div>
                                              )}
                                            </div>
                                          ))}
                                        </div>
                                      );
                                    })}
                                  </div>
                                )}
                              </div>
                            );
                          })}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
