"use client";

import { ReactNode } from "react";
import { LiveRunProvider } from "@/lib/live-run-context";

export function Providers({ children }: { children: ReactNode }) {
  return <LiveRunProvider>{children}</LiveRunProvider>;
}
