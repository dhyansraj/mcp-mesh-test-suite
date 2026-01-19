import { Header } from "@/components/layout/Header";
import { RunsList } from "./RunsList";
import { getRuns } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function RunsPage() {
  const runsData = await getRuns(50, 0).catch(() => ({
    runs: [],
    count: 0,
    limit: 50,
    offset: 0,
  }));

  return (
    <div className="flex flex-col">
      <Header title="Runs" subtitle="Test run history" />

      <div className="flex-1 p-6">
        <RunsList initialRuns={runsData.runs} />
      </div>
    </div>
  );
}
