"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Run,
  formatDuration,
  formatRelativeTime,
  getStatusBgColor,
  rerunFromRun,
  cancelRun,
} from "@/lib/api";
import {
  CheckCircle,
  XCircle,
  Clock,
  ChevronRight,
  Filter,
  RotateCcw,
  Loader2,
  StopCircle,
} from "lucide-react";

interface RunsListProps {
  initialRuns: Run[];
}

type StatusFilter = "all" | "completed" | "failed" | "running" | "cancelled";

export function RunsList({ initialRuns }: RunsListProps) {
  const router = useRouter();
  const [filter, setFilter] = useState<StatusFilter>("all");
  const [rerunningId, setRerunningId] = useState<string | null>(null);
  const [cancellingId, setCancellingId] = useState<string | null>(null);

  const handleRerun = async (e: React.MouseEvent, run: Run) => {
    e.preventDefault(); // Prevent Link navigation
    e.stopPropagation();

    if (!run.suite_id) {
      console.error("Cannot rerun: no suite_id");
      return;
    }

    setRerunningId(run.run_id);
    try {
      const result = await rerunFromRun(run);
      // Navigate to live feed to watch the new run
      router.push("/live");
    } catch (error) {
      console.error("Failed to rerun:", error);
    } finally {
      setRerunningId(null);
    }
  };

  const handleCancel = async (e: React.MouseEvent, run: Run) => {
    e.preventDefault();
    e.stopPropagation();

    setCancellingId(run.run_id);
    try {
      await cancelRun(run.run_id);
      router.refresh();
    } catch (error) {
      console.error("Failed to cancel:", error);
    } finally {
      setCancellingId(null);
    }
  };

  const filteredRuns = initialRuns.filter((run) => {
    if (filter === "all") return true;
    return run.status === filter;
  });

  const filters: { label: string; value: StatusFilter }[] = [
    { label: "All", value: "all" },
    { label: "Completed", value: "completed" },
    { label: "Failed", value: "failed" },
    { label: "Running", value: "running" },
    { label: "Cancelled", value: "cancelled" },
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
                        <span className="text-sm font-medium text-foreground">
                          {run.display_name
                            ? `${run.suite_name} / ${run.display_name}`
                            : run.suite_name || `Run ${run.run_id.slice(0, 8)}`}
                        </span>
                        <Badge
                          variant="secondary"
                          className={`text-xs ${getStatusBgColor(
                            run.cancel_requested && run.status === "running"
                              ? "cancelled"
                              : run.status
                          )}`}
                        >
                          {run.cancel_requested && run.status === "running"
                            ? "cancelling"
                            : run.status}
                        </Badge>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        <span className="font-mono">{run.run_id.slice(0, 8)}</span>
                        {" • "}
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

                    {/* Cancel button - show for running/pending runs */}
                    {(run.status === "running" || run.status === "pending") && (
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10"
                        onClick={(e) => handleCancel(e, run)}
                        disabled={cancellingId === run.run_id || run.cancel_requested}
                        title={run.cancel_requested ? "Cancelling..." : "Cancel"}
                      >
                        {cancellingId === run.run_id ? (
                          <Loader2 className="h-4 w-4 animate-spin" />
                        ) : (
                          <StopCircle className="h-4 w-4" />
                        )}
                      </Button>
                    )}

                    {/* Rerun button */}
                    {run.suite_id && (
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={(e) => handleRerun(e, run)}
                        disabled={rerunningId === run.run_id}
                        title="Rerun"
                      >
                        {rerunningId === run.run_id ? (
                          <Loader2 className="h-4 w-4 animate-spin" />
                        ) : (
                          <RotateCcw className="h-4 w-4" />
                        )}
                      </Button>
                    )}

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
