"use client";

import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Run, formatDuration, formatRelativeTime, getStatusBgColor } from "@/lib/api";
import { CheckCircle, XCircle, Clock, ArrowRight } from "lucide-react";

interface RecentRunsProps {
  runs: Run[];
}

export function RecentRuns({ runs }: RecentRunsProps) {
  return (
    <Card className="border-border bg-card rounded-md">
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="text-lg font-semibold">Recent Runs</CardTitle>
        <Link
          href="/runs"
          className="flex items-center gap-1 text-sm text-primary hover:underline"
        >
          View all <ArrowRight className="h-4 w-4" />
        </Link>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow className="border-border hover:bg-transparent">
              <TableHead className="text-muted-foreground">Run ID</TableHead>
              <TableHead className="text-muted-foreground">Status</TableHead>
              <TableHead className="text-muted-foreground">Tests</TableHead>
              <TableHead className="text-muted-foreground">Duration</TableHead>
              <TableHead className="text-muted-foreground">Started</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {runs.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className="py-8 text-center text-muted-foreground"
                >
                  No runs found
                </TableCell>
              </TableRow>
            ) : (
              runs.map((run) => (
                <TableRow
                  key={run.run_id}
                  className="border-border hover:bg-muted/50"
                >
                  <TableCell>
                    <Link
                      href={`/runs/${run.run_id}`}
                      className="font-mono text-sm text-primary hover:underline"
                    >
                      {run.run_id.slice(0, 8)}...
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant="secondary"
                      className={getStatusBgColor(run.status)}
                    >
                      {run.status}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <span className="flex items-center gap-1 text-success">
                        <CheckCircle className="h-3 w-3" />
                        {run.passed}
                      </span>
                      <span className="text-muted-foreground">/</span>
                      <span className="flex items-center gap-1 text-destructive">
                        <XCircle className="h-3 w-3" />
                        {run.failed}
                      </span>
                      <span className="text-muted-foreground">
                        of {run.total_tests}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1 text-muted-foreground">
                      <Clock className="h-3 w-3" />
                      {formatDuration(run.duration_ms)}
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatRelativeTime(run.started_at)}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
