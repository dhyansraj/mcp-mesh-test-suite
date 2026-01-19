"use client";

import { useState } from "react";
import Link from "next/link";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Run,
  formatDuration,
  formatRelativeTime,
  getStatusBgColor,
} from "@/lib/api";
import {
  CheckCircle,
  XCircle,
  Clock,
  ChevronRight,
  Filter,
} from "lucide-react";

interface RunsListProps {
  initialRuns: Run[];
}

type StatusFilter = "all" | "completed" | "failed" | "running";

export function RunsList({ initialRuns }: RunsListProps) {
  const [filter, setFilter] = useState<StatusFilter>("all");

  const filteredRuns = initialRuns.filter((run) => {
    if (filter === "all") return true;
    return run.status === filter;
  });

  const filters: { label: string; value: StatusFilter }[] = [
    { label: "All", value: "all" },
    { label: "Completed", value: "completed" },
    { label: "Failed", value: "failed" },
    { label: "Running", value: "running" },
  ];

  return (
    <div className="space-y-4">
      {/* Filters */}
      <div className="flex items-center gap-2">
        <Filter className="h-4 w-4 text-muted-foreground" />
        <div className="flex gap-2">
          {filters.map((f) => (
            <Button
              key={f.value}
              variant={filter === f.value ? "default" : "outline"}
              size="sm"
              onClick={() => setFilter(f.value)}
              className={
                filter === f.value
                  ? "bg-primary text-primary-foreground"
                  : "border-border text-muted-foreground hover:text-foreground"
              }
            >
              {f.label}
            </Button>
          ))}
        </div>
      </div>

      {/* Runs List */}
      <div className="flex flex-col gap-3">
        {filteredRuns.length === 0 ? (
          <Card className="border-border bg-card">
            <CardContent className="flex items-center justify-center py-12">
              <p className="text-muted-foreground">No runs found</p>
            </CardContent>
          </Card>
        ) : (
          filteredRuns.map((run) => (
            <Link key={run.run_id} href={`/runs/${run.run_id}`}>
              <Card className="border-border bg-card rounded-md transition-colors hover:bg-muted/30">
                <CardContent className="flex items-center justify-between px-4 py-2.5">
                  <div className="flex items-center gap-3">
                    {/* Status indicator */}
                    <div
                      className={`flex h-8 w-8 items-center justify-center rounded ${
                        run.status === "completed" && run.failed === 0
                          ? "bg-success/20"
                          : run.status === "failed" || run.failed > 0
                          ? "bg-destructive/20"
                          : run.status === "running"
                          ? "bg-primary/20"
                          : "bg-muted"
                      }`}
                    >
                      {run.status === "completed" && run.failed === 0 ? (
                        <CheckCircle className="h-4 w-4 text-success" />
                      ) : run.status === "failed" || run.failed > 0 ? (
                        <XCircle className="h-4 w-4 text-destructive" />
                      ) : run.status === "running" ? (
                        <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                      ) : (
                        <Clock className="h-4 w-4 text-muted-foreground" />
                      )}
                    </div>

                    {/* Run info */}
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-sm font-medium text-foreground">
                          {run.run_id.slice(0, 12)}...
                        </span>
                        <Badge
                          variant="secondary"
                          className={`text-xs ${getStatusBgColor(run.status)}`}
                        >
                          {run.status}
                        </Badge>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {formatRelativeTime(run.started_at)}
                        {run.cli_version && ` • v${run.cli_version}`}
                      </p>
                    </div>
                  </div>

                  <div className="flex items-center gap-6">
                    {/* Test counts */}
                    <div className="flex items-center gap-4 text-sm">
                      <span className="flex items-center gap-1 text-success">
                        <CheckCircle className="h-4 w-4" />
                        {run.passed}
                      </span>
                      <span className="flex items-center gap-1 text-destructive">
                        <XCircle className="h-4 w-4" />
                        {run.failed}
                      </span>
                      <span className="text-muted-foreground">
                        / {run.total_tests}
                      </span>
                    </div>

                    {/* Duration */}
                    <div className="flex items-center gap-1 text-sm text-muted-foreground">
                      <Clock className="h-4 w-4" />
                      {formatDuration(run.duration_ms)}
                    </div>

                    <ChevronRight className="h-5 w-5 text-muted-foreground" />
                  </div>
                </CardContent>
              </Card>
            </Link>
          ))
        )}
      </div>
    </div>
  );
}
