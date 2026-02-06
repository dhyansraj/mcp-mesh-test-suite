"use client";

import { useState, useEffect } from "react";
import {
  ChevronDown,
  ChevronRight,
  Folder,
  FolderOpen,
  FileText,
  Loader2,
  Ban,
  Settings,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { getSuite, SuiteWithTests, SuiteTest } from "@/lib/api";

interface TestCaseTreeProps {
  suiteId: number;
  onSelectTest: (testId: string, testName: string) => void;
  onSelectUC?: (ucName: string) => void;
  selectedTestId?: string;
  selectedUCName?: string;
}

interface UseCaseGroup {
  use_case: string;
  tests: SuiteTest[];
}

function groupTestsByUseCase(tests: SuiteTest[]): UseCaseGroup[] {
  const groups = new Map<string, SuiteTest[]>();

  for (const test of tests) {
    const uc = test.use_case || "unknown";
    if (!groups.has(uc)) {
      groups.set(uc, []);
    }
    groups.get(uc)!.push(test);
  }

  return Array.from(groups.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([use_case, tests]) => ({
      use_case,
      tests: tests.sort((a, b) => a.test_case.localeCompare(b.test_case)),
    }));
}

export function TestCaseTree({
  suiteId,
  onSelectTest,
  onSelectUC,
  selectedTestId,
  selectedUCName,
}: TestCaseTreeProps) {
  const [suite, setSuite] = useState<SuiteWithTests | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedUcs, setExpandedUcs] = useState<Set<string>>(new Set());

  useEffect(() => {
    loadSuite();
  }, [suiteId]);

  const loadSuite = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getSuite(suiteId);
      setSuite(data);
      // Auto-expand all use cases initially
      const ucs = new Set(data.tests?.map((t) => t.use_case) || []);
      setExpandedUcs(ucs);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load suite");
    } finally {
      setLoading(false);
    }
  };

  const toggleUc = (uc: string) => {
    setExpandedUcs((prev) => {
      const next = new Set(prev);
      if (next.has(uc)) {
        next.delete(uc);
      } else {
        next.add(uc);
      }
      return next;
    });
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center py-8 text-destructive text-sm">{error}</div>
    );
  }

  if (!suite || !suite.tests || suite.tests.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground text-sm">
        No tests found in this suite
      </div>
    );
  }

  const useCases = groupTestsByUseCase(suite.tests);

  return (
    <div className="space-y-1 overflow-hidden">
      {useCases.map((uc) => {
        const isExpanded = expandedUcs.has(uc.use_case);
        const allDisabled = uc.tests.length > 0 && uc.tests.every((t) => t.disabled);
        const isUCSelected = selectedUCName === uc.use_case;

        return (
          <div key={uc.use_case} className="min-w-0">
            {/* Use Case Header */}
            <div
              role="button"
              tabIndex={0}
              onClick={() => toggleUc(uc.use_case)}
              className={cn(
                "flex items-center gap-2 w-full p-2 text-left rounded-md transition-colors cursor-pointer",
                isUCSelected
                  ? "bg-primary/10 text-primary"
                  : "hover:bg-muted/50"
              )}
            >
              {isExpanded ? (
                <ChevronDown className="h-4 w-4 text-muted-foreground shrink-0" />
              ) : (
                <ChevronRight className="h-4 w-4 text-muted-foreground shrink-0" />
              )}
              {isExpanded ? (
                <FolderOpen className="h-4 w-4 text-amber-500 shrink-0" />
              ) : (
                <Folder className="h-4 w-4 text-amber-500 shrink-0" />
              )}
              <span className="font-medium text-sm flex-1 truncate min-w-0">
                {uc.use_case.replace(/_/g, " ")}
              </span>
              {allDisabled && (
                <Badge variant="outline" className="text-[10px] border-yellow-500/50 text-yellow-500 px-1.5 py-0">
                  <Ban className="h-3 w-3 mr-0.5" />
                  disabled
                </Badge>
              )}
              {onSelectUC && (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onSelectUC(uc.use_case);
                  }}
                  className="p-1 rounded hover:bg-muted/80 transition-colors"
                  title="UC settings"
                >
                  <Settings className="h-3.5 w-3.5 text-muted-foreground" />
                </button>
              )}
              <span className="text-xs text-muted-foreground">
                {uc.tests.length}
              </span>
            </div>

            {/* Test Cases */}
            {isExpanded && (
              <div className="ml-4 border-l border-border pl-2">
                {uc.tests.map((test) => {
                  const isSelected = selectedTestId === test.test_id;

                  return (
                    <button
                      key={test.test_id}
                      onClick={() =>
                        onSelectTest(test.test_id, test.name || test.test_case)
                      }
                      className={cn(
                        "flex items-center gap-2 w-full p-2 text-left rounded-md transition-colors",
                        isSelected
                          ? "bg-primary/10 text-primary"
                          : "hover:bg-muted/50",
                        test.disabled && "opacity-50"
                      )}
                    >
                      <FileText
                        className={cn(
                          "h-4 w-4",
                          isSelected ? "text-primary" : "text-muted-foreground"
                        )}
                      />
                      <span className="text-sm truncate flex-1">
                        {test.name || test.test_case}
                      </span>
                      {test.disabled && (
                        <Badge variant="outline" className="text-[10px] border-yellow-500/50 text-yellow-500 px-1.5 py-0">
                          <Ban className="h-3 w-3 mr-0.5" />
                          disabled
                        </Badge>
                      )}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
