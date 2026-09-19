import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { toast } from "sonner";
import {
  Activity,
  BarChart3,
  Calendar,
  Coins,
  Cpu,
  FileText,
  Gauge,
  Layers,
  MessageSquare,
  RotateCw,
  Search,
  Sidebar,
  TrendingUp,
  Zap,
} from "lucide-react";
import { ApiError, apiFetch } from "@/lib/api";
import type { UsageSummary } from "@/lib/types";
import { kindDescription, kindLabel } from "@/lib/usageKinds";
import { useSidebar } from "@/components/layout/SidebarContext";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { cn, fmtTokens } from "@/lib/utils";

// 4 dp matches backend micro-USD rounding
const money = (v: number) => {
  if (v === 0) return "$0.00";
  if (v < 0.01) return `$${v.toFixed(4)}`;
  return `$${v.toFixed(2)}`;
};

const exactMoney = (v: number) => `$${v.toFixed(4)}`;

function formatChartDay(dayStr?: unknown): string {
  if (typeof dayStr !== "string") return "";
  try {
    const parts = dayStr.split("-");
    if (parts.length === 3) {
      const d = new Date(Number(parts[0]), Number(parts[1]) - 1, Number(parts[2]));
      return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
    }
    return dayStr;
  } catch {
    return dayStr;
  }
}

function getKindBadgeClass(kind: string): string {
  switch (kind.toLowerCase()) {
    case "chat":
      return "border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-400";
    case "embedding":
      return "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400";
    case "rerank":
      return "border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400";
    case "compaction":
      return "border-purple-500/30 bg-purple-500/10 text-purple-600 dark:text-purple-400";
    default:
      return "border-border bg-muted/60 text-muted-foreground";
  }
}

function getKindBarColor(kind: string): string {
  switch (kind.toLowerCase()) {
    case "chat":
      return "#3b82f6";
    case "embedding":
      return "#10b981";
    case "rerank":
      return "#f59e0b";
    case "compaction":
      return "#a855f7";
    default:
      return "#6b7280";
  }
}

interface StatCardProps {
  label: string;
  value: string;
  subtext?: string;
  icon: React.ReactNode;
  badge?: string;
}

function StatCard({ label, value, subtext, icon, badge }: StatCardProps) {
  return (
    <Card className="relative overflow-hidden border border-border/80 bg-card/60 shadow-xs backdrop-blur-xs transition-all hover:border-border hover:shadow-sm">
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
          {label}
        </CardTitle>
        <div className="flex size-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
          {icon}
        </div>
      </CardHeader>
      <CardContent className="space-y-1">
        <div className="flex items-baseline gap-2">
          <div className="text-2xl font-bold tracking-tight tabular-nums text-foreground">{value}</div>
          {badge && (
            <Badge variant="secondary" className="text-[10px] font-mono px-1.5 py-0 h-4">
              {badge}
            </Badge>
          )}
        </div>
        {subtext && <p className="text-[11px] text-muted-foreground truncate">{subtext}</p>}
      </CardContent>
    </Card>
  );
}

export function UsagePage() {
  const { toggleSidebar, setMobileOpen } = useSidebar();
  const [days, setDays] = useState<number>(30);
  const [chartMetric, setChartMetric] = useState<"cost" | "tokens">("cost");
  const [tableFilter, setTableFilter] = useState("");

  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["usage", days],
    queryFn: () => apiFetch<UsageSummary>(`/api/usage/summary?days=${days}`),
  });

  useEffect(() => {
    if (isError) toast.error(error instanceof ApiError ? error.message : "Failed to load usage");
  }, [isError, error]);

  // Aggregate metrics
  const totals = useMemo(() => {
    if (!data) return { cost: 0, inputTokens: 0, outputTokens: 0, totalTokens: 0, requests: 0, avgPerReq: 0 };
    const cost = data.totals.reduce((a, t) => a + t.costUsd, 0);
    const inputTokens = data.totals.reduce((a, t) => a + t.inputTokens, 0);
    const outputTokens = data.totals.reduce((a, t) => a + t.outputTokens, 0);
    const requests = data.totals.reduce((a, t) => a + t.requests, 0);
    const totalTokens = inputTokens + outputTokens;
    const avgPerReq = requests > 0 ? cost / requests : 0;
    return { cost, inputTokens, outputTokens, totalTokens, requests, avgPerReq };
  }, [data]);

  // Breakdown by kind (category)
  const kindBreakdown = useMemo(() => {
    if (!data) return [];
    const map = new Map<string, { kind: string; cost: number; inputTokens: number; outputTokens: number; requests: number }>();
    for (const t of data.totals) {
      const cur = map.get(t.kind) || { kind: t.kind, cost: 0, inputTokens: 0, outputTokens: 0, requests: 0 };
      cur.cost += t.costUsd;
      cur.inputTokens += t.inputTokens;
      cur.outputTokens += t.outputTokens;
      cur.requests += t.requests;
      map.set(t.kind, cur);
    }
    return Array.from(map.values()).sort((a, b) => b.cost - a.cost);
  }, [data]);

  // Breakdown by model
  const modelBreakdown = useMemo(() => {
    if (!data) return [];
    const map = new Map<string, { model: string; cost: number; requests: number; tokens: number }>();
    for (const t of data.totals) {
      const cur = map.get(t.model) || { model: t.model, cost: 0, requests: 0, tokens: 0 };
      cur.cost += t.costUsd;
      cur.requests += t.requests;
      cur.tokens += t.inputTokens + t.outputTokens;
      map.set(t.model, cur);
    }
    return Array.from(map.values()).sort((a, b) => b.cost - a.cost);
  }, [data]);

  // Filtered table rows
  const filteredRows = useMemo(() => {
    if (!data) return [];
    if (!tableFilter.trim()) return data.totals;
    const q = tableFilter.toLowerCase().trim();
    return data.totals.filter(
      (t) =>
        t.model.toLowerCase().includes(q) ||
        t.kind.toLowerCase().includes(q) ||
        kindLabel(t.kind).toLowerCase().includes(q)
    );
  }, [data, tableFilter]);

  const hasUsage = data && (data.totals.length > 0 || data.daily.length > 0);

  return (
    <div className="flex h-full flex-col bg-background">
      {/* Header bar */}
      <header className="flex items-center justify-between gap-3 border-b border-border/60 px-4 py-2.5 bg-background/80 backdrop-blur-xs z-10 shrink-0">
        <div className="flex items-center gap-2.5 min-w-0">
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => {
              if (window.innerWidth < 768) {
                setMobileOpen(true);
              } else {
                toggleSidebar();
              }
            }}
            aria-label="Toggle sidebar"
            title="Toggle sidebar (⌘B)"
            className="text-muted-foreground hover:text-foreground"
          >
            <Sidebar className="size-4" />
          </Button>
          <div className="flex items-center gap-2">
            <Gauge className="size-4 text-primary" />
            <h1 className="text-sm font-semibold tracking-tight text-foreground">Usage & Cost Ledger</h1>
          </div>
        </div>

        {/* Time period filter pills & refresh */}
        <div className="flex items-center gap-2">
          <div className="flex items-center rounded-lg border border-border/60 bg-muted/40 p-0.5 text-xs font-medium">
            {[
              { label: "7d", value: 7 },
              { label: "30d", value: 30 },
              { label: "90d", value: 90 },
            ].map((p) => (
              <button
                key={p.value}
                type="button"
                onClick={() => setDays(p.value)}
                className={cn(
                  "rounded-md px-2.5 py-1 text-xs font-medium transition-all",
                  days === p.value
                    ? "bg-background text-foreground shadow-xs font-semibold"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                {p.label}
              </button>
            ))}
          </div>

          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => void refetch()}
            disabled={isFetching}
            title="Refresh usage data"
            aria-label="Refresh usage data"
            className="text-muted-foreground hover:text-foreground"
          >
            <RotateCw className={cn("size-3.5", isFetching && "animate-spin text-primary")} />
          </Button>
        </div>
      </header>

      {/* Main content area */}
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-6xl space-y-6 p-4 sm:p-6 lg:p-8">
          {/* Subtitle / Description bar */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
            <div>
              <h2 className="text-xl font-bold tracking-tight text-foreground">Resource Consumption</h2>
              <p className="text-xs text-muted-foreground">
                Real-time accounting of tokens, API requests, and model expenses for the past {days} days.
              </p>
            </div>
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Calendar className="size-3.5" />
              <span>Last {days} days</span>
            </div>
          </div>

          {/* Loading Skeletons */}
          {isLoading && (
            <div className="space-y-6">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                {[0, 1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-24 rounded-xl" />
                ))}
              </div>
              <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
                <Skeleton className="h-[340px] rounded-xl lg:col-span-8" />
                <Skeleton className="h-[340px] rounded-xl lg:col-span-4" />
              </div>
              <Skeleton className="h-64 rounded-xl w-full" />
            </div>
          )}

          {/* Error State */}
          {isError && (
            <Card className="border-destructive/40 bg-destructive/5 p-6 text-center">
              <div className="space-y-3">
                <p className="text-destructive font-medium text-sm">Failed to load usage statistics.</p>
                <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
                  Try again
                </Button>
              </div>
            </Card>
          )}

          {/* Loaded Content */}
          {data && (
            <>
              {/* Top Metrics Cards */}
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <StatCard
                  label="Total Spend"
                  value={money(totals.cost)}
                  subtext={`$${totals.cost.toFixed(4)} total ledger`}
                  icon={<Coins className="size-4" />}
                />
                <StatCard
                  label="Tokens Processed"
                  value={fmtTokens(totals.totalTokens)}
                  subtext={`${totals.inputTokens.toLocaleString()} in / ${totals.outputTokens.toLocaleString()} out`}
                  icon={<Zap className="size-4" />}
                />
                <StatCard
                  label="API Invocations"
                  value={totals.requests.toLocaleString()}
                  subtext="Model & tool generation turns"
                  icon={<Activity className="size-4" />}
                />
                <StatCard
                  label="Avg. Cost / Request"
                  value={totals.avgPerReq > 0 ? exactMoney(totals.avgPerReq) : "$0.0000"}
                  subtext="Per-request unit cost"
                  icon={<TrendingUp className="size-4" />}
                />
              </div>

              {!hasUsage ? (
                /* Empty state */
                <Card className="py-12 text-center border-dashed">
                  <CardContent className="flex flex-col items-center justify-center space-y-4">
                    <div className="flex size-14 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
                      <BarChart3 className="size-7" />
                    </div>
                    <div className="space-y-1 max-w-sm">
                      <h3 className="font-semibold text-base text-foreground">No usage recorded yet</h3>
                      <p className="text-xs text-muted-foreground">
                        Usage tokens and expenditure will automatically populate here once you start chatting or running workflows.
                      </p>
                    </div>
                    <Button asChild size="sm" variant="brand" className="gap-2">
                      <Link to="/chat">
                        <MessageSquare className="size-4" />
                        <span>Start a chat</span>
                      </Link>
                    </Button>
                  </CardContent>
                </Card>
              ) : (
                /* Two-Column Dashboard Layout (Main + Side) */
                <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
                  {/* MAIN COLUMN (8 cols) */}
                  <div className="space-y-6 lg:col-span-8">
                    {/* Interactive Trends Chart */}
                    <Card className="border border-border/80 bg-card/60 shadow-xs">
                      <CardHeader className="flex flex-row items-center justify-between pb-2 border-b border-border/40">
                        <div>
                          <CardTitle className="text-sm font-semibold tracking-tight">
                            Daily Activity & Spend
                          </CardTitle>
                          <CardDescription className="text-xs">
                            {chartMetric === "cost" ? "Daily expenditure in USD" : "Daily token throughput"}
                          </CardDescription>
                        </div>
                        {/* Toggle between Cost and Tokens */}
                        <div className="flex items-center rounded-lg border border-border/60 bg-muted/40 p-0.5 text-xs font-medium">
                          <button
                            type="button"
                            onClick={() => setChartMetric("cost")}
                            className={cn(
                              "rounded-md px-2.5 py-0.5 text-xs transition-all",
                              chartMetric === "cost"
                                ? "bg-background text-foreground shadow-xs font-semibold"
                                : "text-muted-foreground hover:text-foreground"
                            )}
                          >
                            Cost ($)
                          </button>
                          <button
                            type="button"
                            onClick={() => setChartMetric("tokens")}
                            className={cn(
                              "rounded-md px-2.5 py-0.5 text-xs transition-all",
                              chartMetric === "tokens"
                                ? "bg-background text-foreground shadow-xs font-semibold"
                                : "text-muted-foreground hover:text-foreground"
                            )}
                          >
                            Tokens
                          </button>
                        </div>
                      </CardHeader>
                      <CardContent className="pt-4">
                        <div className="h-[280px] w-full">
                          <ResponsiveContainer width="100%" height="100%">
                            {chartMetric === "cost" ? (
                              <BarChart data={data.daily} margin={{ top: 10, right: 10, left: -15, bottom: 0 }}>
                                <CartesianGrid strokeDasharray="3 3" vertical={false} opacity={0.3} />
                                <XAxis
                                  dataKey="day"
                                  tickLine={false}
                                  axisLine={false}
                                  fontSize={11}
                                  tickFormatter={formatChartDay}
                                />
                                <YAxis
                                  tickLine={false}
                                  axisLine={false}
                                  fontSize={11}
                                  width={60}
                                  tickFormatter={(v) => money(Number(v))}
                                />
                                <Tooltip
                                  formatter={(v) => [exactMoney(Number(v)), "Cost"]}
                                  labelFormatter={formatChartDay}
                                  contentStyle={{
                                    backgroundColor: "var(--color-card, #18181b)",
                                    borderColor: "var(--color-border, #27272a)",
                                    borderRadius: "0.75rem",
                                    fontSize: "0.75rem",
                                    boxShadow: "0 4px 12px rgba(0,0,0,0.15)",
                                  }}
                                  cursor={{ fill: "var(--accent)", opacity: 0.2 }}
                                />
                                <Bar dataKey="costUsd" fill="var(--color-primary, #3b82f6)" radius={[4, 4, 0, 0]} />
                              </BarChart>
                            ) : (
                              <BarChart data={data.daily} margin={{ top: 10, right: 10, left: -15, bottom: 0 }}>
                                <CartesianGrid strokeDasharray="3 3" vertical={false} opacity={0.3} />
                                <XAxis
                                  dataKey="day"
                                  tickLine={false}
                                  axisLine={false}
                                  fontSize={11}
                                  tickFormatter={formatChartDay}
                                />
                                <YAxis
                                  tickLine={false}
                                  axisLine={false}
                                  fontSize={11}
                                  width={60}
                                  tickFormatter={(v) => fmtTokens(Number(v))}
                                />
                                <Tooltip
                                  formatter={(v, name) => [
                                    Number(v).toLocaleString(),
                                    name === "inputTokens" ? "Input Tokens" : "Output Tokens",
                                  ]}
                                  labelFormatter={formatChartDay}
                                  contentStyle={{
                                    backgroundColor: "var(--color-card, #18181b)",
                                    borderColor: "var(--color-border, #27272a)",
                                    borderRadius: "0.75rem",
                                    fontSize: "0.75rem",
                                    boxShadow: "0 4px 12px rgba(0,0,0,0.15)",
                                  }}
                                  cursor={{ fill: "var(--accent)", opacity: 0.2 }}
                                />
                                <Legend
                                  verticalAlign="top"
                                  align="right"
                                  iconSize={8}
                                  wrapperStyle={{ fontSize: "11px", paddingBottom: "10px" }}
                                />
                                <Bar
                                  dataKey="inputTokens"
                                  name="Input Tokens"
                                  stackId="a"
                                  fill="#3b82f6"
                                  radius={[0, 0, 0, 0]}
                                />
                                <Bar
                                  dataKey="outputTokens"
                                  name="Output Tokens"
                                  stackId="a"
                                  fill="#06b6d4"
                                  radius={[4, 4, 0, 0]}
                                />
                              </BarChart>
                            )}
                          </ResponsiveContainer>
                        </div>
                      </CardContent>
                    </Card>

                    {/* Detailed Consumption Table */}
                    <Card className="border border-border/80 bg-card/60 shadow-xs">
                      <CardHeader className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 border-b border-border/40">
                        <div>
                          <CardTitle className="text-sm font-semibold tracking-tight">
                            Activity Ledger
                          </CardTitle>
                          <CardDescription className="text-xs">
                            Breakdown by model and execution type
                          </CardDescription>
                        </div>

                        {/* Search input */}
                        <div className="relative w-full sm:w-56">
                          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
                          <input
                            type="text"
                            value={tableFilter}
                            onChange={(e) => setTableFilter(e.target.value)}
                            placeholder="Filter models or kind…"
                            className="h-8 w-full rounded-lg border border-border/80 bg-background/50 pl-8 pr-3 text-xs outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                          />
                        </div>
                      </CardHeader>

                      <div className="overflow-x-auto">
                        <table className="w-full text-xs">
                          <thead className="bg-muted/40 text-left text-muted-foreground font-medium border-b border-border/40">
                            <tr>
                              <th className="px-4 py-2.5">Model / Engine</th>
                              <th className="px-4 py-2.5">Kind</th>
                              <th className="px-4 py-2.5 text-right">Input Tokens</th>
                              <th className="px-4 py-2.5 text-right">Output Tokens</th>
                              <th className="px-4 py-2.5 text-right">Requests</th>
                              <th className="px-4 py-2.5 text-right">Cost</th>
                              <th className="px-4 py-2.5 text-right">Share</th>
                            </tr>
                          </thead>
                          <tbody className="divide-y divide-border/40">
                            {filteredRows.length === 0 ? (
                              <tr>
                                <td colSpan={7} className="px-4 py-8 text-center text-muted-foreground text-xs">
                                  No records matching "{tableFilter}".
                                </td>
                              </tr>
                            ) : (
                              filteredRows.map((t) => {
                                const sharePct = totals.cost > 0 ? (t.costUsd / totals.cost) * 100 : 0;
                                return (
                                  <tr
                                    key={`${t.kind}:${t.model}`}
                                    className="hover:bg-muted/30 transition-colors"
                                  >
                                    <td className="px-4 py-2.5 font-medium text-foreground flex items-center gap-1.5">
                                      <Cpu className="size-3 text-muted-foreground shrink-0" />
                                      <span className="font-mono text-[11px] truncate max-w-[140px] sm:max-w-[200px]">
                                        {t.model}
                                      </span>
                                    </td>
                                    <td className="px-4 py-2.5">
                                      <Badge
                                        variant="outline"
                                        className={cn("text-[10px] font-mono px-2 py-0.5", getKindBadgeClass(t.kind))}
                                        title={kindDescription(t.kind) || undefined}
                                      >
                                        {kindLabel(t.kind)}
                                      </Badge>
                                    </td>
                                    <td className="px-4 py-2.5 text-right font-mono tabular-nums text-muted-foreground">
                                      {t.inputTokens.toLocaleString()}
                                    </td>
                                    <td className="px-4 py-2.5 text-right font-mono tabular-nums text-muted-foreground">
                                      {t.outputTokens.toLocaleString()}
                                    </td>
                                    <td className="px-4 py-2.5 text-right font-mono tabular-nums text-muted-foreground">
                                      {t.requests.toLocaleString()}
                                    </td>
                                    <td className="px-4 py-2.5 text-right font-semibold font-mono tabular-nums text-foreground">
                                      {exactMoney(t.costUsd)}
                                    </td>
                                    <td className="px-4 py-2.5 text-right font-mono text-[11px] text-muted-foreground">
                                      {sharePct.toFixed(1)}%
                                    </td>
                                  </tr>
                                );
                              })
                            )}
                          </tbody>
                        </table>
                      </div>
                    </Card>
                  </div>

                  {/* SIDE PANEL (4 cols) */}
                  <div className="space-y-6 lg:col-span-4">
                    {/* Category (Kind) Spend Breakdown */}
                    <Card className="border border-border/80 bg-card/60 shadow-xs">
                      <CardHeader className="pb-3 border-b border-border/40">
                        <div className="flex items-center gap-2">
                          <Layers className="size-4 text-primary" />
                          <CardTitle className="text-sm font-semibold tracking-tight">
                            Spend by Category
                          </CardTitle>
                        </div>
                        <CardDescription className="text-xs">
                          Distribution across core AI services
                        </CardDescription>
                      </CardHeader>
                      <CardContent className="pt-4 space-y-3.5">
                        {kindBreakdown.length === 0 ? (
                          <p className="text-xs text-muted-foreground">No category data.</p>
                        ) : (
                          kindBreakdown.map((k) => {
                            const pct = totals.cost > 0 ? (k.cost / totals.cost) * 100 : 0;
                            const barColor = getKindBarColor(k.kind);
                            return (
                              <div key={k.kind} className="space-y-1.5">
                                <div className="flex items-center justify-between text-xs">
                                  <div className="flex items-center gap-1.5">
                                    <span
                                      className="size-2 rounded-full"
                                      style={{ backgroundColor: barColor }}
                                    />
                                    <span className="font-medium">{kindLabel(k.kind)}</span>
                                  </div>
                                  <div className="flex items-center gap-2 font-mono text-[11px]">
                                    <span className="text-foreground font-semibold">{exactMoney(k.cost)}</span>
                                    <span className="text-muted-foreground">({pct.toFixed(0)}%)</span>
                                  </div>
                                </div>
                                <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                                  <div
                                    className="h-full rounded-full transition-all duration-300"
                                    style={{
                                      width: `${Math.max(pct, 2)}%`,
                                      backgroundColor: barColor,
                                    }}
                                  />
                                </div>
                              </div>
                            );
                          })
                        )}
                      </CardContent>
                    </Card>

                    {/* Top Models by Spend */}
                    <Card className="border border-border/80 bg-card/60 shadow-xs">
                      <CardHeader className="pb-3 border-b border-border/40">
                        <div className="flex items-center gap-2">
                          <Cpu className="size-4 text-primary" />
                          <CardTitle className="text-sm font-semibold tracking-tight">
                            Top Models
                          </CardTitle>
                        </div>
                        <CardDescription className="text-xs">
                          Ranked by cumulative spend
                        </CardDescription>
                      </CardHeader>
                      <CardContent className="pt-4 space-y-3">
                        {modelBreakdown.slice(0, 5).map((m, idx) => {
                          const pct = totals.cost > 0 ? (m.cost / totals.cost) * 100 : 0;
                          return (
                            <div key={m.model} className="space-y-1 py-1 border-b border-border/20 last:border-0 text-xs">
                              <div className="flex items-center justify-between">
                                <div className="flex items-center gap-2 min-w-0 flex-1 pr-2">
                                  <span className="flex size-4.5 items-center justify-center rounded-full bg-muted font-mono text-[10px] text-muted-foreground shrink-0">
                                    {idx + 1}
                                  </span>
                                  <span className="font-mono text-[11px] truncate font-medium" title={m.model}>
                                    {m.model}
                                  </span>
                                </div>
                                <div className="text-right shrink-0">
                                  <div className="font-mono font-semibold text-foreground text-[11px]">
                                    {exactMoney(m.cost)}
                                  </div>
                                  <div className="text-[10px] text-muted-foreground">
                                    {m.requests} reqs ({pct.toFixed(0)}%)
                                  </div>
                                </div>
                              </div>
                              <div className="h-1 w-full overflow-hidden rounded-full bg-muted/60">
                                <div
                                  className="h-full rounded-full bg-primary/70 transition-all duration-300"
                                  style={{ width: `${Math.max(pct, 2)}%` }}
                                />
                              </div>
                            </div>
                          );
                        })}
                      </CardContent>
                    </Card>

                    {/* Document Embeddings Knowledge Base */}
                    {data.documents && data.documents.length > 0 && (
                      <Card className="border border-border/80 bg-card/60 shadow-xs">
                        <CardHeader className="pb-3 border-b border-border/40">
                          <div className="flex items-center justify-between">
                            <div className="flex items-center gap-2">
                              <FileText className="size-4 text-primary" />
                              <CardTitle className="text-sm font-semibold tracking-tight">
                                Document Indexing
                              </CardTitle>
                            </div>
                            <Badge variant="secondary" className="font-mono text-[10px]">
                              {data.documents.length} docs
                            </Badge>
                          </div>
                          <CardDescription className="text-xs">
                            Embedding vector storage spend
                          </CardDescription>
                        </CardHeader>
                        <CardContent className="pt-3 max-h-64 overflow-y-auto space-y-2">
                          {data.documents.map((d) => (
                            <div
                              key={d.documentId}
                              className="flex items-center justify-between rounded-lg p-2 text-xs bg-muted/30 hover:bg-muted/50 transition-colors"
                            >
                              <div className="flex items-center gap-2 min-w-0 flex-1 pr-2">
                                <FileText className="size-3.5 text-primary/70 shrink-0" />
                                <span className="truncate font-medium text-foreground text-[11px]" title={d.filename}>
                                  {d.filename}
                                </span>
                              </div>
                              <div className="text-right shrink-0">
                                <div className="font-mono font-semibold text-foreground text-[11px]">
                                  {exactMoney(d.costUsd)}
                                </div>
                                <div className="text-[10px] font-mono text-muted-foreground">
                                  {fmtTokens(d.inputTokens)} tokens
                                </div>
                              </div>
                            </div>
                          ))}
                        </CardContent>
                      </Card>
                    )}

                    {/* Token Ratio Summary */}
                    <Card className="border border-border/80 bg-card/60 shadow-xs p-4">
                      <div className="space-y-2">
                        <div className="flex items-center justify-between text-xs">
                          <span className="font-semibold text-muted-foreground">Prompt / Completion Ratio</span>
                          <span className="font-mono text-[11px]">
                            {totals.totalTokens > 0
                              ? `${((totals.inputTokens / totals.totalTokens) * 100).toFixed(0)}% in / ${((totals.outputTokens / totals.totalTokens) * 100).toFixed(0)}% out`
                              : "N/A"}
                          </span>
                        </div>
                        <div className="flex h-2 w-full overflow-hidden rounded-full bg-muted">
                          <div
                            className="bg-blue-500 transition-all"
                            style={{
                              width: `${totals.totalTokens > 0 ? (totals.inputTokens / totals.totalTokens) * 100 : 50}%`,
                            }}
                            title="Input tokens"
                          />
                          <div
                            className="bg-cyan-500 transition-all"
                            style={{
                              width: `${totals.totalTokens > 0 ? (totals.outputTokens / totals.totalTokens) * 100 : 50}%`,
                            }}
                            title="Output tokens"
                          />
                        </div>
                        <p className="text-[10px] text-muted-foreground/80 leading-relaxed pt-1">
                          Herbie automatically tracks token consumption per session, helping you optimize workflows and avoid unexpected model inference expenses.
                        </p>
                      </div>
                    </Card>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
