"use client";

import { createContext, useContext, ReactNode } from "react";
import { useSSE } from "./sse";

interface LiveRunContextType {
  currentRunId: string | null;
  connected: boolean;
}

const LiveRunContext = createContext<LiveRunContextType>({
  currentRunId: null,
  connected: false,
});

export function LiveRunProvider({ children }: { children: ReactNode }) {
  const { currentRunId, connected } = useSSE();

  return (
    <LiveRunContext.Provider value={{ currentRunId, connected }}>
      {children}
    </LiveRunContext.Provider>
  );
}

export function useLiveRun() {
  return useContext(LiveRunContext);
}
