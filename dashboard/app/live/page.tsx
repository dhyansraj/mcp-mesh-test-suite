"use client";

import { Header } from "@/components/layout/Header";
import { LiveFeed } from "./LiveFeed";

export default function LivePage() {
  return (
    <div className="flex flex-col">
      <Header title="Live View" subtitle="Real-time test execution monitor" />

      <div className="flex-1 p-6">
        <LiveFeed />
      </div>
    </div>
  );
}
