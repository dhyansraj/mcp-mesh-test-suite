"use client";

import { useLiveEvents } from "@/lib/sse";
import { cn } from "@/lib/utils";

interface HeaderProps {
  title: string;
  subtitle?: string;
}

export function Header({ title, subtitle }: HeaderProps) {
  const { connected, currentRunId } = useLiveEvents();

  return (
    <header className="flex h-16 items-center justify-between border-b border-border bg-background px-6">
      <div>
        <h1 className="text-xl font-semibold text-foreground">{title}</h1>
        {subtitle && (
          <p className="text-sm text-muted-foreground">{subtitle}</p>
        )}
      </div>

      <div className="flex items-center gap-4">
        {/* Connection Status */}
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "flex h-2 w-2 rounded-full",
              connected ? "bg-success" : "bg-destructive"
            )}
          />
          <span className="text-sm text-muted-foreground">
            {connected ? "Connected" : "Disconnected"}
          </span>
        </div>

        {/* Current Run Indicator */}
        {currentRunId && (
          <div className="flex items-center gap-2 rounded-lg bg-primary/10 px-3 py-1">
            <span className="relative flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75"></span>
              <span className="relative inline-flex h-2 w-2 rounded-full bg-primary"></span>
            </span>
            <span className="text-sm font-medium text-primary">
              Run: {currentRunId.slice(0, 8)}...
            </span>
          </div>
        )}
      </div>
    </header>
  );
}
