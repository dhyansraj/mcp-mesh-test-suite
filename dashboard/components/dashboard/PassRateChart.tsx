"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { Run } from "@/lib/api";

interface PassRateChartProps {
  runs: Run[];
}

export function PassRateChart({ runs }: PassRateChartProps) {
  // Process runs to create chart data (last 30 runs, reversed for chronological order)
  const chartData = runs
    .slice(0, 30)
    .reverse()
    .map((run, index) => {
      const total = run.passed + run.failed;
      const passRate = total > 0 ? (run.passed / total) * 100 : 0;
      return {
        index: index + 1,
        passRate: Math.round(passRate * 10) / 10,
        passed: run.passed,
        failed: run.failed,
        date: run.started_at
          ? new Date(run.started_at).toLocaleDateString("en-US", {
              month: "short",
              day: "numeric",
            })
          : `Run ${index + 1}`,
      };
    });

  return (
    <Card className="border-border bg-card rounded-md">
      <CardHeader>
        <CardTitle className="text-lg font-semibold">
          Pass Rate Trend
        </CardTitle>
      </CardHeader>
      <CardContent>
        {chartData.length === 0 ? (
          <div className="flex h-[200px] items-center justify-center text-muted-foreground">
            No data available
          </div>
        ) : (
          <ResponsiveContainer width="100%" height={200}>
            <AreaChart data={chartData}>
              <defs>
                <linearGradient id="passRateGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#22d3ee" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#22d3ee" stopOpacity={0} />
                </linearGradient>
              </defs>
              <XAxis
                dataKey="date"
                stroke="#94a3b8"
                fontSize={12}
                tickLine={false}
                axisLine={false}
              />
              <YAxis
                stroke="#94a3b8"
                fontSize={12}
                tickLine={false}
                axisLine={false}
                domain={[0, 100]}
                tickFormatter={(value) => `${value}%`}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: "#0f2744",
                  border: "1px solid #1e3a5f",
                  borderRadius: "8px",
                }}
                labelStyle={{ color: "#f1f5f9" }}
                itemStyle={{ color: "#22d3ee" }}
                formatter={(value) => [`${value}%`, "Pass Rate"]}
              />
              <Area
                type="monotone"
                dataKey="passRate"
                stroke="#22d3ee"
                strokeWidth={2}
                fill="url(#passRateGradient)"
              />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </CardContent>
    </Card>
  );
}
