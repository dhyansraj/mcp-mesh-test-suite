import { Header } from "@/components/layout/Header";
import { StatsCards } from "@/components/dashboard/StatsCards";
import { RecentRuns } from "@/components/dashboard/RecentRuns";
import { PassRateChart } from "@/components/dashboard/PassRateChart";
import { getRuns, getStats } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  // Fetch data in parallel
  const [runsData, stats] = await Promise.all([
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

  return (
    <div className="flex flex-col">
      <Header title="Dashboard" subtitle="Overview of test runs and metrics" />

      <div className="flex-1 space-y-6 p-6">
        {/* Stats Cards */}
        <StatsCards
          totalRuns={stats.total_runs}
          passRate={stats.pass_rate}
          avgDuration={stats.avg_run_duration_ms}
          totalTests={stats.total_tests_executed}
        />

        {/* Charts and Recent Runs */}
        <div className="grid gap-6 lg:grid-cols-2">
          <PassRateChart runs={runsData.runs} />
          <RecentRuns runs={runsData.runs} />
        </div>
      </div>
    </div>
  );
}
