import { Header } from "@/components/layout/Header";
import { SettingsContent } from "./SettingsContent";
import { getSuites } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function SettingsPage() {
  const suitesData = await getSuites().catch(() => ({
    suites: [],
    count: 0,
  }));

  return (
    <div className="flex flex-col">
      <Header title="Settings" subtitle="Manage test suites" />

      <div className="flex-1 p-6">
        <SettingsContent initialSuites={suitesData.suites} />
      </div>
    </div>
  );
}
