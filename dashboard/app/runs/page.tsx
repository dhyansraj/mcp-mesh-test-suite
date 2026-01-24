"use client";

import { useEffect, useState, Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { Header } from "@/components/layout/Header";
import { RunsList } from "./RunsList";
import { RunDetails } from "./RunDetails";
import { getRuns, getRun, getRunTests, Run, RunSummary, TestResult } from "@/lib/api";
import { Loader2, AlertCircle, ArrowLeft } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import Link from "next/link";

function RunsContent() {
  const searchParams = useSearchParams();
  const runId = searchParams.get("id");

  // List state
  const [runs, setRuns] = useState<Run[]>([]);
  const [listLoading, setListLoading] = useState(true);

  // Detail state
  const [run, setRun] = useState<RunSummary | null>(null);
  const [tests, setTests] = useState<TestResult[]>([]);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);

  // Fetch list on mount
  useEffect(() => {
    async function fetchList() {
      try {
        const runsData = await getRuns(50, 0).catch(() => ({
          runs: [],
          count: 0,
          limit: 50,
          offset: 0,
        }));
        setRuns(runsData.runs);
      } finally {
        setListLoading(false);
      }
    }
    fetchList();
  }, []);

  // Fetch detail when runId changes
  useEffect(() => {
    if (!runId) {
      setRun(null);
      setTests([]);
      setDetailError(null);
      return;
    }

    async function fetchDetail() {
      if (!runId) return;
      setDetailLoading(true);
      setDetailError(null);
      try {
        const [runData, testsData] = await Promise.all([
          getRun(runId),
          getRunTests(runId),
        ]);
        setRun(runData);
        setTests(testsData.tests);
      } catch (err) {
        setDetailError("Run not found");
      } finally {
        setDetailLoading(false);
      }
    }
    fetchDetail();
  }, [runId]);

  // Show detail view
  if (runId) {
    if (detailLoading) {
      return (
        <div className="flex flex-col">
          <Header title="Run Details" subtitle="Loading..." />
          <div className="flex-1 flex items-center justify-center p-6">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </div>
        </div>
      );
    }

    if (detailError || !run) {
      return (
        <div className="flex flex-col">
          <Header title="Run Details" subtitle="Error" />
          <div className="flex-1 p-6">
            <Card>
              <CardContent className="flex flex-col items-center justify-center py-12">
                <AlertCircle className="h-12 w-12 text-destructive mb-4" />
                <p className="text-lg font-medium">Run not found</p>
                <p className="text-sm text-muted-foreground mt-2">
                  The run with ID {runId?.slice(0, 8)}... does not exist.
                </p>
                <Button asChild variant="outline" className="mt-4">
                  <Link href="/runs">
                    <ArrowLeft className="h-4 w-4 mr-2" />
                    Back to Runs
                  </Link>
                </Button>
              </CardContent>
            </Card>
          </div>
        </div>
      );
    }

    return (
      <div className="flex flex-col">
        <Header
          title={run.display_name
            ? `${run.suite_name} / ${run.display_name}`
            : run.suite_name || `Run ${runId.slice(0, 8)}`}
          subtitle={`${runId.slice(0, 8)} • ${run.total_tests} tests • ${run.status}`}
        />

        <div className="flex-1 p-6">
          <div className="mb-4">
            <Button asChild variant="ghost" size="sm">
              <Link href="/runs">
                <ArrowLeft className="h-4 w-4 mr-2" />
                Back to Runs
              </Link>
            </Button>
          </div>
          <RunDetails run={run} tests={tests} />
        </div>
      </div>
    );
  }

  // Show list view
  if (listLoading) {
    return (
      <div className="flex flex-col">
        <Header title="Runs" subtitle="Test run history" />
        <div className="flex-1 flex items-center justify-center p-6">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col">
      <Header title="Runs" subtitle="Test run history" />

      <div className="flex-1 p-6">
        <RunsList initialRuns={runs} />
      </div>
    </div>
  );
}

export default function RunsPage() {
  return (
    <Suspense fallback={
      <div className="flex flex-col">
        <Header title="Runs" subtitle="Test run history" />
        <div className="flex-1 flex items-center justify-center p-6">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      </div>
    }>
      <RunsContent />
    </Suspense>
  );
}
