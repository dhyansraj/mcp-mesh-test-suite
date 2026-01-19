import { Header } from "@/components/layout/Header";
import { RunDetails } from "./RunDetails";
import { getRun, getRunTests } from "@/lib/api";
import { notFound } from "next/navigation";

export const dynamic = "force-dynamic";

interface Props {
  params: Promise<{ id: string }>;
}

export default async function RunDetailPage({ params }: Props) {
  const { id } = await params;

  try {
    const [run, testsData] = await Promise.all([
      getRun(id),
      getRunTests(id),
    ]);

    return (
      <div className="flex flex-col">
        <Header
          title={`Run ${id}`}
          subtitle={`${run.total_tests} tests • ${run.status}`}
        />

        <div className="flex-1 p-6">
          <RunDetails run={run} tests={testsData.tests} />
        </div>
      </div>
    );
  } catch {
    notFound();
  }
}
