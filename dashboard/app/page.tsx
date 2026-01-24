"use client";

import { useEffect, useState } from "react";
import { Header } from "@/components/layout/Header";
import { StatsCards } from "@/components/dashboard/StatsCards";
import { RecentRuns } from "@/components/dashboard/RecentRuns";
import { PassRateChart } from "@/components/dashboard/PassRateChart";
import { getRuns, getStats, Run, Stats } from "@/lib/api";
import { Loader2 } from "lucide-react";

export default function DashboardPage() {
  const [runs, setRuns] = useState<Run[]>([]);
  const [stats, setStats] = useState<Stats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchData() {
      try {
        const [runsData, statsData] = await Promise.all([
          getRuns(10, 0).catch(() => ({ runs: [], count: 0, limit: 10, offset: 0 })),
          getStats().catch(() => ({
            total_runs: 0,
            total_tests_executed: 0,
            total_passed: 0,
            total_failed: 0,
            avg_run_duration_ms: null,
            pass_rate: 0,
          })),
        ]);
        setRuns(runsData.runs);
        setStats(statsData);
      } finally {
        setLoading(false);
      }
    }
    fetchData();
  }, []);

  if (loading) {
    return (
      <div className="flex flex-col">
        <Header title="Dashboard" subtitle="Overview of test runs and metrics" />
        <div className="flex-1 flex items-center justify-center p-6">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col">
      <Header title="Dashboard" subtitle="Overview of test runs and metrics" />

      <div className="flex-1 space-y-6 p-6">
        {/* Stats Cards */}
        <StatsCards
          totalRuns={stats?.total_runs ?? 0}
          passRate={stats?.pass_rate ?? 0}
          avgDuration={stats?.avg_run_duration_ms ?? null}
          totalTests={stats?.total_tests_executed ?? 0}
        />

        {/* Charts and Recent Runs */}
        <div className="grid gap-6 lg:grid-cols-2">
          <PassRateChart runs={runs} />
          <RecentRuns runs={runs} />
        </div>
      </div>
    </div>
  );
}
