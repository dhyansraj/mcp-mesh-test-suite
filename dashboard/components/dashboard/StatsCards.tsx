"use client";

import { Card, CardContent } from "@/components/ui/card";
import { Activity, CheckCircle, Clock, TestTube } from "lucide-react";
import { formatDuration } from "@/lib/api";

interface StatsCardsProps {
  totalRuns: number;
  passRate: number;
  avgDuration: number | null;
  totalTests: number;
}

export function StatsCards({
  totalRuns,
  passRate,
  avgDuration,
  totalTests,
}: StatsCardsProps) {
  const stats = [
    {
      name: "Total Runs",
      value: totalRuns.toString(),
      icon: Activity,
      color: "text-primary",
      bgColor: "bg-primary/10",
    },
    {
      name: "Pass Rate",
      value: `${passRate.toFixed(1)}%`,
      icon: CheckCircle,
      color: "text-success",
      bgColor: "bg-success/10",
    },
    {
      name: "Avg Duration",
      value: formatDuration(avgDuration),
      icon: Clock,
      color: "text-accent",
      bgColor: "bg-accent/10",
    },
    {
      name: "Total Tests",
      value: totalTests.toString(),
      icon: TestTube,
      color: "text-secondary",
      bgColor: "bg-secondary/10",
    },
  ];

  return (
    <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-4">
      {stats.map((stat) => (
        <Card key={stat.name} className="border-border bg-card rounded-md">
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-muted-foreground">
                  {stat.name}
                </p>
                <p className="mt-2 text-3xl font-bold text-foreground">
                  {stat.value}
                </p>
              </div>
              <div className={`rounded p-3 ${stat.bgColor}`}>
                <stat.icon className={`h-6 w-6 ${stat.color}`} />
              </div>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
