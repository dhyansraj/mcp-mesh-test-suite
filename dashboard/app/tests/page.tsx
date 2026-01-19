import { Header } from "@/components/layout/Header";
import { TestsBrowser } from "./TestsBrowser";
import { getSuites } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function TestsPage() {
  const suitesData = await getSuites().catch(() => ({
    suites: [],
    count: 0,
  }));

  return (
    <div className="flex flex-col">
      <Header title="Tests" subtitle="Browse test cases by suite" />

      <div className="flex-1 p-6">
        <TestsBrowser suites={suitesData.suites} />
      </div>
    </div>
  );
}
