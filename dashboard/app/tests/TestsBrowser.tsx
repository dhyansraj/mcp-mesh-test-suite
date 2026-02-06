"use client";

import { useState, useMemo, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  ChevronRight,
  ChevronDown,
  FolderOpen,
  Folder,
  FileText,
  Container,
  Terminal,
  FlaskConical,
  Play,
  Loader2,
  X,
  Square,
  CheckSquare,
  MinusSquare,
  Filter,
  Search,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { Suite, SuiteTest, getSuite, runTests } from "@/lib/api";

// Consistent tag color palette - 6 harmonious colors
const TAG_COLORS = [
  "bg-blue-500/15 text-blue-400 border-blue-500/20",
  "bg-emerald-500/15 text-emerald-400 border-emerald-500/20",
  "bg-violet-500/15 text-violet-400 border-violet-500/20",
  "bg-amber-500/15 text-amber-400 border-amber-500/20",
  "bg-rose-500/15 text-rose-400 border-rose-500/20",
  "bg-cyan-500/15 text-cyan-400 border-cyan-500/20",
];

function getTagColor(tag: string): string {
  let hash = 0;
  for (let i = 0; i < tag.length; i++) {
    hash = ((hash << 5) - hash + tag.charCodeAt(i)) | 0;
  }
  return TAG_COLORS[Math.abs(hash) % TAG_COLORS.length];
}

interface TestsBrowserProps {
  suites: Suite[];
}

interface UseCaseGroup {
  useCase: string;
  testCases: TestCaseGroup[];
  disabled: boolean;
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

  return Array.from(useCaseMap.entries()).map(([useCase, testCaseMap]) => {
    const testCases = Array.from(testCaseMap.entries()).map(([testCase, tests]) => ({
      testCase,
      tests,
    }));
    const allDisabled = testCases.every(tc => tc.tests.every(t => t.disabled));
    return {
      useCase,
      testCases,
      disabled: allDisabled && testCases.length > 0,
    };
  });
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

  // Tag filtering state
  const [selectedTags, setSelectedTags] = useState<Set<string>>(new Set());

  // Selection state - tracks selected test IDs (format: "uc/tc")
  const [selectedTests, setSelectedTests] = useState<Set<string>>(new Set());

  // Running state for selected tests
  const [runningSelected, setRunningSelected] = useState(false);

  // Filter panel visibility state
  const [showFilterPanel, setShowFilterPanel] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");

  // Helper to check if filters are active
  const hasActiveFilters = selectedTags.size > 0;

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

  // Extract all unique tags from all loaded tests
  const allTags = useMemo(() => {
    const tags = new Set<string>();
    suiteTests.forEach((tests) => {
      tests.forEach((test) => {
        if (!test.disabled) {
          test.tags.forEach((tag) => tags.add(tag));
        }
      });
    });
    return Array.from(tags).sort();
  }, [suiteTests]);

  const tagCounts = useMemo(() => {
    const counts = new Map<string, number>();
    suiteTests.forEach((tests) => {
      tests.forEach((test) => {
        if (!test.disabled) {
          test.tags.forEach((tag) => {
            counts.set(tag, (counts.get(tag) || 0) + 1);
          });
        }
      });
    });
    return counts;
  }, [suiteTests]);

  // Check if a test passes the tag filter
  const testPassesFilter = (test: SuiteTest): boolean => {
    const passesTag = selectedTags.size === 0 || Array.from(selectedTags).every((tag) => test.tags.includes(tag));
    const passesSearch = !searchQuery ||
      (test.name || "").toLowerCase().includes(searchQuery.toLowerCase()) ||
      test.test_id.toLowerCase().includes(searchQuery.toLowerCase());
    return passesTag && passesSearch;
  };

  // Toggle tag selection
  const toggleTag = (tag: string) => {
    setSelectedTags((prev) => {
      const next = new Set(prev);
      if (next.has(tag)) {
        next.delete(tag);
      } else {
        next.add(tag);
      }
      return next;
    });
  };

  // Auto-select matching tests when tags or search change
  useEffect(() => {
    if (selectedTags.size > 0 || searchQuery) {
      const matchingIds: string[] = [];
      suiteTests.forEach((tests) => {
        tests.forEach((test) => {
          if (testPassesFilter(test) && !test.disabled) {
            matchingIds.push(test.test_id);
          }
        });
      });
      setSelectedTests(new Set(matchingIds));
    } else {
      setSelectedTests(new Set());
    }
  }, [selectedTags, searchQuery, suiteTests]);

  // Get all tests that match the current filter (flat list)
  const getFilteredTestsList = useMemo(() => {
    if (selectedTags.size === 0 && !searchQuery) return [];

    const matchingTests: Array<{ test: SuiteTest; suiteId: number; suiteName: string }> = [];

    suites.forEach((suite) => {
      const tests = suiteTests.get(suite.id) || [];
      tests.forEach((test) => {
        if (testPassesFilter(test) && !test.disabled) {
          matchingTests.push({ test, suiteId: suite.id, suiteName: suite.suite_name });
        }
      });
    });

    return matchingTests;
  }, [selectedTags, searchQuery, suiteTests, suites]);

  const allFilteredSelected = useMemo(() => {
    const allFilteredIds: string[] = [];
    suiteTests.forEach((tests) => {
      tests.filter(t => testPassesFilter(t) && !t.disabled).forEach((t) => allFilteredIds.push(t.test_id));
    });
    return allFilteredIds.length > 0 && allFilteredIds.every((id) => selectedTests.has(id));
  }, [suiteTests, selectedTests, selectedTags, searchQuery]);

  // Clear all selected tags
  const clearTags = () => {
    setSelectedTags(new Set());
  };

  // Toggle test selection
  const toggleTestSelection = (testId: string) => {
    // Find if this test is disabled
    let isDisabled = false;
    suiteTests.forEach((tests) => {
      const test = tests.find(t => t.test_id === testId);
      if (test?.disabled) isDisabled = true;
    });
    if (isDisabled) return;

    setSelectedTests((prev) => {
      const next = new Set(prev);
      if (next.has(testId)) {
        next.delete(testId);
      } else {
        next.add(testId);
      }
      return next;
    });
  };

  // Select/deselect all tests in a test case
  const toggleTcSelection = (tests: SuiteTest[]) => {
    const testIds = tests.filter(t => testPassesFilter(t) && !t.disabled).map((t) => t.test_id);
    setSelectedTests((prev) => {
      const next = new Set(prev);
      const allSelected = testIds.every((id) => prev.has(id));
      if (allSelected) {
        testIds.forEach((id) => next.delete(id));
      } else {
        testIds.forEach((id) => next.add(id));
      }
      return next;
    });
  };

  // Select/deselect all tests in a use case
  const toggleUcSelection = (testCases: TestCaseGroup[]) => {
    const testIds = testCases.flatMap((tc) =>
      tc.tests.filter(t => testPassesFilter(t) && !t.disabled).map((t) => t.test_id)
    );
    setSelectedTests((prev) => {
      const next = new Set(prev);
      const allSelected = testIds.every((id) => prev.has(id));
      if (allSelected) {
        testIds.forEach((id) => next.delete(id));
      } else {
        testIds.forEach((id) => next.add(id));
      }
      return next;
    });
  };

  // Get checkbox state for a test case
  const getTcCheckboxState = (tests: SuiteTest[]): "none" | "some" | "all" => {
    const filteredTests = tests.filter(t => testPassesFilter(t) && !t.disabled);
    if (filteredTests.length === 0) return "none";
    const selectedCount = filteredTests.filter((t) => selectedTests.has(t.test_id)).length;
    if (selectedCount === 0) return "none";
    if (selectedCount === filteredTests.length) return "all";
    return "some";
  };

  // Get checkbox state for a use case
  const getUcCheckboxState = (testCases: TestCaseGroup[]): "none" | "some" | "all" => {
    const allTests = testCases.flatMap((tc) => tc.tests.filter(t => testPassesFilter(t) && !t.disabled));
    if (allTests.length === 0) return "none";
    const selectedCount = allTests.filter((t) => selectedTests.has(t.test_id)).length;
    if (selectedCount === 0) return "none";
    if (selectedCount === allTests.length) return "all";
    return "some";
  };

  // Select all filtered tests across all loaded suites
  const selectAllFiltered = () => {
    const allFilteredIds: string[] = [];
    suiteTests.forEach((tests) => {
      tests.filter(t => testPassesFilter(t) && !t.disabled).forEach((t) => allFilteredIds.push(t.test_id));
    });
    setSelectedTests(new Set(allFilteredIds));
  };

  // Clear all selections
  const clearSelection = () => {
    setSelectedTests(new Set());
  };

  // Run selected tests
  const handleRunSelected = async () => {
    if (selectedTests.size === 0) return;

    // Find which suite(s) the selected tests belong to
    // For now, assume tests come from a single expanded suite
    const expandedSuiteId = Array.from(expandedSuites)[0];
    if (!expandedSuiteId) return;

    setRunningSelected(true);
    setRunMessage(null);
    try {
      await runTests(expandedSuiteId, {
        test_ids: Array.from(selectedTests),
        tags: selectedTags.size > 0 ? Array.from(selectedTags) : undefined
      });
      router.push("/live");
    } catch (err) {
      setRunMessage(
        `Error: ${err instanceof Error ? err.message : "Failed to start"}`
      );
      setTimeout(() => setRunMessage(null), 5000);
    } finally {
      setRunningSelected(false);
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
          <div className="flex items-center justify-between">
            <CardTitle className="text-lg font-medium flex items-center gap-2">
              <FlaskConical className="h-5 w-5" />
              Test Browser
            </CardTitle>
            <Button
              variant={hasActiveFilters ? "default" : "outline"}
              size="sm"
              className={cn(
                "h-8 gap-2",
                hasActiveFilters && !showFilterPanel && "bg-primary text-primary-foreground",
                showFilterPanel && "bg-muted"
              )}
              onClick={() => setShowFilterPanel(!showFilterPanel)}
            >
              <Filter className="h-4 w-4" />
              {hasActiveFilters && (
                <span className="text-xs">
                  {selectedTags.size} tag{selectedTags.size !== 1 ? 's' : ''}
                </span>
              )}
            </Button>
          </div>
        </CardHeader>

        {/* Search Bar */}
        <div className="px-6 pb-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search tests by name..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 pr-9 h-9 bg-muted/30 border-border"
            />
            {searchQuery && (
              <button
                onClick={() => setSearchQuery("")}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
              >
                <X className="h-4 w-4" />
              </button>
            )}
          </div>
        </div>

        {/* Filter Panel - collapsible */}
        {showFilterPanel && (
          <div className="px-6 pb-4 space-y-3 border-b">
            {/* Tag Filter */}
            {allTags.length > 0 && (
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium text-muted-foreground">Tags:</span>
                {allTags.map((tag) => (
                  <Badge
                    key={tag}
                    variant={selectedTags.has(tag) ? "default" : "outline"}
                    className={cn(
                      "cursor-pointer transition-colors relative transition-transform duration-150 hover:scale-[1.35] hover:z-10",
                      selectedTags.has(tag)
                        ? "bg-primary hover:bg-primary/80"
                        : "hover:bg-muted"
                    )}
                    onClick={() => toggleTag(tag)}
                  >
                    {tag} <span className="ml-1 opacity-60">({tagCounts.get(tag) || 0})</span>
                  </Badge>
                ))}
                {selectedTags.size > 0 && (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 px-2 text-xs"
                    onClick={clearTags}
                  >
                    <X className="h-3 w-3 mr-1" />
                    Clear
                  </Button>
                )}
              </div>
            )}

            {/* Selection Bar */}
            {suiteTests.size > 0 && (
              <div className="flex items-center gap-4 pt-3 mt-1 border-t border-border/50">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 text-xs border-primary/40 text-primary hover:bg-primary/10"
                  onClick={allFilteredSelected ? clearSelection : selectAllFiltered}
                >
                  {allFilteredSelected ? (
                    <><Square className="h-3 w-3 mr-1" />Deselect All</>
                  ) : (
                    <><CheckSquare className="h-3 w-3 mr-1" />Select All</>
                  )}
                </Button>
                <div className="flex-1 text-sm text-muted-foreground">
                  {`${selectedTests.size} tests selected`}
                </div>
                <Button
                  variant="default"
                  size="sm"
                  className="h-7 bg-green-600 hover:bg-green-700"
                  onClick={handleRunSelected}
                  disabled={selectedTests.size === 0 || runningSelected}
                >
                  {runningSelected ? (
                    <Loader2 className="h-3 w-3 mr-1 animate-spin" />
                  ) : (
                    <Play className="h-3 w-3 mr-1" />
                  )}
                  Run Selected
                </Button>
              </div>
            )}
          </div>
        )}

        <CardContent>
          {(selectedTags.size > 0 || searchQuery) ? (
            // Flat list view when tags or search are active
            <div className="space-y-1">
              {getFilteredTestsList.length === 0 ? (
                <div className="p-4 text-sm text-muted-foreground text-center">
                  No tests match the selected tags
                </div>
              ) : (
                getFilteredTestsList.map(({ test, suiteId, suiteName }) => {
                  const isSelected = selectedTests.has(test.test_id);
                  return (
                    <div
                      key={`${suiteId}-${test.test_id}`}
                      className="flex items-center gap-3 p-2 rounded-md hover:bg-muted/30 transition-colors"
                    >
                      <div
                        className="cursor-pointer"
                        onClick={() => toggleTestSelection(test.test_id)}
                      >
                        {isSelected ? (
                          <CheckSquare className="h-4 w-4 text-primary" />
                        ) : (
                          <Square className="h-4 w-4 text-muted-foreground" />
                        )}
                      </div>
                      <FileText className="h-4 w-4 text-muted-foreground" />
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-medium truncate">
                          {test.name || test.test_id}
                        </div>
                        <div className="text-xs text-muted-foreground truncate">
                          {suiteName} / {test.test_id}
                        </div>
                      </div>
                      <div className="flex items-center gap-1 flex-shrink-0">
                        {test.tags.map((tag, tagIndex) => (
                          <Badge
                            key={`${test.test_id}-${tag}-${tagIndex}`}
                            variant="outline"
                            className={cn(
                              "text-[10px] px-1.5 py-0",
                              selectedTags.has(tag) ? "bg-primary/20 text-primary border-primary/30" : getTagColor(tag)
                            )}
                          >
                            {tag}
                          </Badge>
                        ))}
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-6 w-6 text-green-500 hover:text-green-600 hover:bg-green-500/10 flex-shrink-0"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleRunTc(e, suiteId, test.test_id);
                        }}
                        title={`Run ${test.test_id}`}
                      >
                        <Play className="h-3 w-3" />
                      </Button>
                    </div>
                  );
                })
              )}
            </div>
          ) : (
            // Tree view when no tags selected
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
                        {tests.filter(t => t.disabled).length > 0 && (
                          <span className="text-yellow-500 ml-1">
                            ({tests.filter(t => t.disabled).length} disabled)
                          </span>
                        )}
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
                                    className={cn(
                                      "flex items-center gap-2 px-3 py-2 pl-8 hover:bg-muted/50 transition-colors cursor-pointer",
                                      ucGroup.disabled && "opacity-50"
                                    )}
                                    onClick={() => toggleUseCase(ucKey)}
                                  >
                                    <div
                                      className={cn("cursor-pointer", ucGroup.disabled && "pointer-events-none opacity-30")}
                                      onClick={(e) => {
                                        e.stopPropagation();
                                        toggleUcSelection(ucGroup.testCases);
                                      }}
                                    >
                                      {getUcCheckboxState(ucGroup.testCases) === "all" ? (
                                        <CheckSquare className="h-4 w-4 text-primary" />
                                      ) : getUcCheckboxState(ucGroup.testCases) === "some" ? (
                                        <MinusSquare className="h-4 w-4 text-primary" />
                                      ) : (
                                        <Square className="h-4 w-4 text-muted-foreground" />
                                      )}
                                    </div>
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
                                      {ucGroup.disabled && (
                                        <Badge variant="outline" className="text-yellow-500 border-yellow-500/30 text-[10px] px-1.5 py-0">
                                          disabled
                                        </Badge>
                                      )}
                                    </div>
                                    <span className="text-xs text-muted-foreground">
                                      {ucGroup.testCases.reduce(
                                        (sum, tc) => sum + tc.tests.filter(testPassesFilter).length,
                                        0
                                      )}{" "}
                                      tests
                                      {(() => {
                                        const disabledCount = ucGroup.testCases.reduce(
                                          (sum, tc) => sum + tc.tests.filter(t => t.disabled).length,
                                          0
                                        );
                                        return disabledCount > 0 ? (
                                          <span className="text-yellow-500 ml-1">({disabledCount} disabled)</span>
                                        ) : null;
                                      })()}
                                    </span>
                                    <Button
                                      variant="ghost"
                                      size="icon"
                                      className="h-6 w-6 text-green-500 hover:text-green-600 hover:bg-green-500/10"
                                      onClick={(e) =>
                                        handleRunUc(e, suite.id, ucGroup.useCase)
                                      }
                                      disabled={isUcRunning || ucGroup.disabled}
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
                                              <div
                                                className={cn("cursor-pointer", tcGroup.tests.every(t => t.disabled) && "pointer-events-none opacity-30")}
                                                onClick={(e) => {
                                                  e.stopPropagation();
                                                  toggleTcSelection(tcGroup.tests);
                                                }}
                                              >
                                                {getTcCheckboxState(tcGroup.tests) === "all" ? (
                                                  <CheckSquare className="h-3.5 w-3.5 text-primary" />
                                                ) : getTcCheckboxState(tcGroup.tests) === "some" ? (
                                                  <MinusSquare className="h-3.5 w-3.5 text-primary" />
                                                ) : (
                                                  <Square className="h-3.5 w-3.5 text-muted-foreground" />
                                                )}
                                              </div>
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
                                                disabled={isTcRunning || tcGroup.tests.every(t => t.disabled)}
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
                                            {tcGroup.tests.map((test) => {
                                              const passesFilter = testPassesFilter(test);
                                              const isSelected = selectedTests.has(test.test_id);

                                              return (
                                                <div
                                                  key={test.test_id}
                                                  className={cn(
                                                    "flex items-center gap-2 px-3 py-1.5 pl-10 hover:bg-muted/30 transition-colors",
                                                    !passesFilter && "opacity-40",
                                                    test.disabled && "opacity-50"
                                                  )}
                                                >
                                                  <div
                                                    className={cn(
                                                      "cursor-pointer",
                                                      !passesFilter && "pointer-events-none",
                                                      test.disabled && "pointer-events-none opacity-30"
                                                    )}
                                                    onClick={() => toggleTestSelection(test.test_id)}
                                                  >
                                                    {isSelected ? (
                                                      <CheckSquare className="h-3.5 w-3.5 text-primary" />
                                                    ) : (
                                                      <Square className="h-3.5 w-3.5 text-muted-foreground" />
                                                    )}
                                                  </div>
                                                  <FileText className="h-3.5 w-3.5 text-muted-foreground" />
                                                  <span className="text-sm flex-1 truncate">
                                                    {test.name || test.test_id}
                                                  </span>
                                                  {test.disabled && (
                                                    <Badge variant="outline" className="text-yellow-500 border-yellow-500/30 text-[10px] px-1.5 py-0">
                                                      disabled
                                                    </Badge>
                                                  )}
                                                  {test.tags.length > 0 && (
                                                    <div className="flex items-center gap-1">
                                                      {test.tags
                                                        .slice(0, 3)
                                                        .map((tag, tagIndex) => (
                                                          <Badge
                                                            key={`${test.test_id}-${tag}-${tagIndex}`}
                                                            variant="outline"
                                                            className={cn(
                                                              "text-[10px] px-1.5 py-0",
                                                              selectedTags.has(tag) ? "bg-primary/20 text-primary border-primary/30" : getTagColor(tag)
                                                            )}
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
                                              );
                                            })}
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
          )}
        </CardContent>
      </Card>
    </div>
  );
}
