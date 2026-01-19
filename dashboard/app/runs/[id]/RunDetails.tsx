"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  RunSummary,
  TestResult,
  formatDuration,
  formatRelativeTime,
  getStatusBgColor,
} from "@/lib/api";
import {
  CheckCircle,
  XCircle,
  Clock,
  ChevronDown,
  ChevronRight,
  Terminal,
  AlertCircle,
} from "lucide-react";

interface RunDetailsProps {
  run: RunSummary;
  tests: TestResult[];
}

export function RunDetails({ run, tests }: RunDetailsProps) {
  const [expandedTests, setExpandedTests] = useState<Set<number>>(new Set());

  const toggleTest = (testId: number) => {
    setExpandedTests((prev) => {
      const next = new Set(prev);
      if (next.has(testId)) {
        next.delete(testId);
      } else {
        next.add(testId);
      }
      return next;
    });
  };

  const passedTests = tests.filter((t) => t.status === "passed");
  const failedTests = tests.filter((t) => t.status === "failed");

  return (
    <div className="space-y-6">
      {/* Run Summary Card */}
      <Card className="border-border bg-card rounded-md">
        <CardContent className="p-6">
          <div className="grid gap-6 md:grid-cols-4">
            <div>
              <p className="text-sm text-muted-foreground">Status</p>
              <Badge
                variant="secondary"
                className={`mt-1 ${getStatusBgColor(run.status)}`}
              >
                {run.status}
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

      {/* Tests Tabs */}
      <Tabs defaultValue="all" className="space-y-4">
        <TabsList className="bg-muted">
          <TabsTrigger value="all">All ({tests.length})</TabsTrigger>
          <TabsTrigger value="passed" className="text-success">
            Passed ({passedTests.length})
          </TabsTrigger>
          <TabsTrigger value="failed" className="text-destructive">
            Failed ({failedTests.length})
          </TabsTrigger>
        </TabsList>

        <TabsContent value="all">
          <TestList
            tests={tests}
            expandedTests={expandedTests}
            onToggle={toggleTest}
          />
        </TabsContent>
        <TabsContent value="passed">
          <TestList
            tests={passedTests}
            expandedTests={expandedTests}
            onToggle={toggleTest}
          />
        </TabsContent>
        <TabsContent value="failed">
          <TestList
            tests={failedTests}
            expandedTests={expandedTests}
            onToggle={toggleTest}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}

interface TestListProps {
  tests: TestResult[];
  expandedTests: Set<number>;
  onToggle: (id: number) => void;
}

function TestList({ tests, expandedTests, onToggle }: TestListProps) {
  if (tests.length === 0) {
    return (
      <Card className="border-border bg-card rounded-md">
        <CardContent className="flex items-center justify-center py-12">
          <p className="text-muted-foreground">No tests found</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      {tests.map((test) => (
        <Card key={test.id} className="border-border bg-card rounded-md">
          <CardHeader
            className="cursor-pointer p-4 hover:bg-muted/30"
            onClick={() => onToggle(test.id)}
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                {expandedTests.has(test.id) ? (
                  <ChevronDown className="h-4 w-4 text-muted-foreground" />
                ) : (
                  <ChevronRight className="h-4 w-4 text-muted-foreground" />
                )}
                {test.status === "passed" ? (
                  <CheckCircle className="h-5 w-5 text-success" />
                ) : test.status === "failed" ? (
                  <XCircle className="h-5 w-5 text-destructive" />
                ) : (
                  <Clock className="h-5 w-5 text-muted-foreground" />
                )}
                <div>
                  <CardTitle className="text-sm font-medium">
                    {test.name}
                  </CardTitle>
                  <p className="font-mono text-xs text-muted-foreground">
                    {test.test_id}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-4">
                {test.tags.length > 0 && (
                  <div className="flex gap-1">
                    {test.tags.slice(0, 3).map((tag) => (
                      <Badge
                        key={tag}
                        variant="outline"
                        className="border-border text-xs"
                      >
                        {tag}
                      </Badge>
                    ))}
                  </div>
                )}
                <div className="flex items-center gap-1 text-sm text-muted-foreground">
                  <Clock className="h-3 w-3" />
                  {formatDuration(test.duration_ms)}
                </div>
              </div>
            </div>
          </CardHeader>

          {expandedTests.has(test.id) && (
            <CardContent className="border-t border-border pt-4">
              {test.error_message && (
                <div className="mb-4 rounded-lg bg-destructive/10 p-4">
                  <div className="flex items-start gap-2">
                    <AlertCircle className="mt-0.5 h-4 w-4 text-destructive" />
                    <div>
                      <p className="text-sm font-medium text-destructive">
                        Error
                      </p>
                      <p className="mt-1 font-mono text-xs text-destructive/80">
                        {test.error_message}
                      </p>
                    </div>
                  </div>
                </div>
              )}

              <div className="space-y-2">
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Terminal className="h-4 w-4" />
                  Test Details
                </div>
                <ScrollArea className="h-[200px] rounded-lg bg-background p-4">
                  <pre className="font-mono text-xs text-muted-foreground">
                    <div>Use Case: {test.use_case}</div>
                    <div>Test Case: {test.test_case}</div>
                    <div>Started: {test.started_at || "N/A"}</div>
                    <div>Finished: {test.finished_at || "N/A"}</div>
                    <div>Duration: {formatDuration(test.duration_ms)}</div>
                  </pre>
                </ScrollArea>
              </div>
            </CardContent>
          )}
        </Card>
      ))}
    </div>
  );
}
