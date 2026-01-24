"use client";

import { useEffect, useState } from "react";
import { Header } from "@/components/layout/Header";
import { TestsBrowser } from "./TestsBrowser";
import { getSuites, Suite } from "@/lib/api";
import { Loader2 } from "lucide-react";

export default function TestsPage() {
  const [suites, setSuites] = useState<Suite[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchData() {
      try {
        const suitesData = await getSuites().catch(() => ({
          suites: [],
          count: 0,
        }));
        setSuites(suitesData.suites);
      } finally {
        setLoading(false);
      }
    }
    fetchData();
  }, []);

  if (loading) {
    return (
      <div className="flex flex-col">
        <Header title="Tests" subtitle="Browse test cases by suite" />
        <div className="flex-1 flex items-center justify-center p-6">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col">
      <Header title="Tests" subtitle="Browse test cases by suite" />

      <div className="flex-1 p-6">
        <TestsBrowser suites={suites} />
      </div>
    </div>
  );
}
