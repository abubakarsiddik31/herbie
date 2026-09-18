import { useEffect } from "react";
import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { toast } from "sonner";
import { ArrowLeft } from "lucide-react";
import { ApiError, apiFetch } from "@/lib/api";
import type { UsageSummary } from "@/lib/types";
import { kindDescription, kindLabel } from "@/lib/usageKinds";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

// 4 dp matches the backend's micro-USD rounding (see internal/httpapi/usage.go).
const money = (v: number) => `$${v.toFixed(4)}`;

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <Card className="gap-2 py-4">
      <CardHeader>
        <CardTitle className="text-muted-foreground text-xs font-medium">{label}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold tabular-nums">{value}</div>
      </CardContent>
    </Card>
  );
}

export function UsagePage() {
  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["usage"],
    queryFn: () => apiFetch<UsageSummary>("/api/usage/summary?days=30"),
  });

  useEffect(() => {
    if (isError) toast.error(error instanceof ApiError ? error.message : "Failed to load usage");
  }, [isError, error]);

  return (
    <div className="mx-auto w-full max-w-4xl space-y-6 p-6">
      <div className="flex items-center gap-1">
        <Button variant="ghost" size="icon-sm" asChild aria-label="Back to chat">
          <Link to="/"><ArrowLeft /></Link>
        </Button>
        <h1 className="text-lg font-semibold">Usage</h1>
      </div>

      {isLoading && (
        <div className="space-y-6">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-28" />)}
          </div>
          <Skeleton className="h-[300px] w-full" />
        </div>
      )}

      {isError && (
        <div className="flex items-center gap-3">
          <p className="text-destructive text-sm">Failed to load usage.</p>
          <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
            Retry
          </Button>
        </div>
      )}

      {data && (
        <>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard label="Total cost" value={money(data.totals.reduce((a, t) => a + t.costUsd, 0))} />
            <StatCard label="Input tokens" value={data.totals.reduce((a, t) => a + t.inputTokens, 0).toLocaleString()} />
            <StatCard label="Output tokens" value={data.totals.reduce((a, t) => a + t.outputTokens, 0).toLocaleString()} />
            <StatCard label="Requests" value={data.totals.reduce((a, t) => a + t.requests, 0).toLocaleString()} />
          </div>

          {data.totals.length === 0 && data.daily.length === 0 ? (
            <p className="text-muted-foreground text-sm">No usage yet — start chatting.</p>
          ) : (
            <>
              <Card className="gap-4 py-4">
                <CardHeader>
                  <CardTitle className="text-sm">Daily cost — last 30 days</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="h-[300px] w-full">
                    <ResponsiveContainer width="100%" height="100%">
                      <BarChart data={data.daily}>
                        <CartesianGrid strokeDasharray="3 3" vertical={false} />
                        <XAxis dataKey="day" tickLine={false} axisLine={false} fontSize={12} />
                        <YAxis tickLine={false} axisLine={false} fontSize={12} width={72}
                          tickFormatter={(v) => money(Number(v))} />
                        <Tooltip formatter={(v) => money(Number(v))} cursor={{ fill: "var(--accent)" }} />
                        <Bar dataKey="costUsd" fill="var(--primary)" radius={[4, 4, 0, 0]} />
                      </BarChart>
                    </ResponsiveContainer>
                  </div>
                </CardContent>
              </Card>

              <div className="overflow-hidden rounded-xl border">
                <table className="w-full text-sm">
                  <thead className="bg-muted/50 text-left text-muted-foreground">
                    <tr>
                      {["Model", "Kind", "Input tokens", "Output tokens", "Requests", "Cost"].map((h) => (
                        <th key={h} className="px-4 py-2.5 font-medium">{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {data.totals.map((t) => (
                      <tr key={`${t.kind}:${t.model}`} className="border-t">
                        <td className="px-4 py-2.5 font-medium">{t.model}</td>
                        <td className="text-muted-foreground px-4 py-2.5" title={kindDescription(t.kind) || undefined}>{kindLabel(t.kind)}</td>
                        <td className="px-4 py-2.5 tabular-nums">{t.inputTokens.toLocaleString()}</td>
                        <td className="px-4 py-2.5 tabular-nums">{t.outputTokens.toLocaleString()}</td>
                        <td className="px-4 py-2.5 tabular-nums">{t.requests.toLocaleString()}</td>
                        <td className="px-4 py-2.5 tabular-nums">{money(t.costUsd)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              {data.documents && data.documents.length > 0 && (
                <div className="overflow-hidden rounded-xl border">
                  <div className="bg-muted/50 px-4 py-2.5 text-sm font-medium">
                    Embedding spend by document — last 30 days
                  </div>
                  <table className="w-full text-sm">
                    <thead className="bg-muted/50 text-left text-muted-foreground">
                      <tr>
                        {["Document", "Embedding tokens", "Cost"].map((h) => (
                          <th key={h} className="px-4 py-2.5 font-medium">{h}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {data.documents.map((d) => (
                        <tr key={d.documentId} className="border-t">
                          <td className="px-4 py-2.5 font-medium">{d.filename}</td>
                          <td className="px-4 py-2.5 tabular-nums">{d.inputTokens.toLocaleString()}</td>
                          <td className="px-4 py-2.5 tabular-nums">{money(d.costUsd)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </>
          )}
        </>
      )}
    </div>
  );
}
