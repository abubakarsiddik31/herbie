import { useEffect, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { MessageSquare, ShieldCheck, Wrench, X } from "lucide-react";
import { ApiError } from "@/lib/api";
import { cn, fmtTokens } from "@/lib/utils";
import type { BotRun, BotRunActivity, BotRunStatus, PendingCall } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { RunLoader } from "@/components/ai/RunLoader";
import { useBotRun, useCancelRun, useResolveRunApprovals } from "@/features/bots/useBots";

// STATUS_STYLES-style map adapted from DocumentsPage: filled badges for the
// in-flight/failure states, plain outline for the settled-neutral ones.
export const RUN_STATUS_STYLES: Record<BotRunStatus, string> = {
  queued: "border-slate-500/30 text-slate-600 dark:text-slate-400",
  running: "border-emerald-600/25 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400",
  waiting_approval: "border-amber-600/25 bg-amber-600/10 text-amber-700 dark:text-amber-400",
  succeeded: "border-emerald-600/30 text-emerald-700 dark:text-emerald-400",
  failed: "border-rose-600/25 bg-rose-600/10 text-rose-700 dark:text-rose-400",
  cancelled: "border-slate-500/30 text-slate-500 dark:text-slate-400",
};

export const RUN_STATUS_LABELS: Record<BotRunStatus, string> = {
  queued: "Queued",
  running: "Running",
  waiting_approval: "Waiting for approval",
  succeeded: "Succeeded",
  failed: "Failed",
  cancelled: "Cancelled",
};

function humanizeActivity(entry: BotRunActivity): string {
  if (entry.event === "tool_start" && entry.name) return `Running ${entry.name}`;
  if (entry.event === "tool_end" && entry.name) {
    return `${entry.name} finished${entry.ok === false ? " (failed)" : ""}`;
  }
  return entry.event.replace(/_/g, " ");
}

/** Ticking "· Ns" measured from startedAt, shown while a run is executing. */
function ElapsedSeconds({ startedAt }: { startedAt: string }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);
  const seconds = Math.max(0, Math.floor((now - new Date(startedAt).getTime()) / 1000));
  return <span className="font-mono text-[10px] text-muted-foreground">· {seconds}s</span>;
}

function ApprovalPanel({
  pending,
  isPending,
  onDecide,
}: {
  pending: PendingCall[];
  isPending: boolean;
  onDecide: (approved: boolean) => void;
}) {
  return (
    <div className="overflow-hidden rounded-xl border border-amber-500/40 bg-amber-500/5">
      <div className="flex items-center gap-2 border-b border-amber-500/20 px-4 py-3">
        <ShieldCheck className="size-4 shrink-0 text-amber-600 dark:text-amber-400" />
        <p className="text-sm font-medium">
          This run wants to execute {pending.length === 1 ? "a tool" : `${pending.length} tools`}
        </p>
      </div>
      <div className="space-y-2 p-3">
        {pending.map((p) => (
          <div key={p.callId} className="rounded-lg border bg-background/70 px-3 py-2.5">
            <Badge variant="outline" className="font-mono text-xs">
              {p.toolName}
            </Badge>
            <pre className="mt-1.5 max-h-40 overflow-auto rounded-md bg-muted/60 p-2 font-mono text-[11px] leading-relaxed text-muted-foreground">
              {JSON.stringify(p.args, null, 2)}
            </pre>
            {p.reason && <p className="mt-1 text-xs text-muted-foreground">{p.reason}</p>}
          </div>
        ))}
      </div>
      <div className="flex gap-2 px-3 pb-3">
        <Button size="sm" onClick={() => onDecide(true)} disabled={isPending}>
          Approve
        </Button>
        <Button size="sm" variant="outline" onClick={() => onDecide(false)} disabled={isPending}>
          Deny
        </Button>
      </div>
    </div>
  );
}

// RunCard renders one BotRun: live status, activity trace, the approval
// panel while it is parked, and a usage footer once evidence lands.
export function RunCard({ run }: { run: BotRun }) {
  const cancel = useCancelRun(run.botId);
  const resolve = useResolveRunApprovals(run.botId);
  const waitingApproval = run.status === "waiting_approval";
  const detail = useBotRun(waitingApproval ? run.id : null);
  const pending = waitingApproval ? (detail.data?.pending ?? []) : [];

  // Cancel stays available while parked: the backend cancels a parked
  // (waiting_approval) run from its queue, so keep the escape hatch.
  const isCancellable =
    run.status === "queued" || run.status === "running" || run.status === "waiting_approval";
  const activity = run.activity.slice(-5);
  const traceNames = run.activity.flatMap((a) => (a.name ? [a.name] : []));

  function handleCancel() {
    cancel.mutate(run.id, {
      onError: (err) =>
        toast.error(err instanceof ApiError ? err.message : "Failed to cancel run"),
    });
  }

  function handleDecide(approved: boolean) {
    resolve.mutate(
      {
        runId: run.id,
        decisions: pending.map((p) => ({ callId: p.callId, approved })),
      },
      {
        onError: (err) =>
          toast.error(err instanceof ApiError ? err.message : "Failed to resolve approval"),
      },
    );
  }

  return (
    <Card className="gap-3 p-4 shadow-xs">
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <Badge
            variant="outline"
            className={cn("shrink-0 text-[10px]", RUN_STATUS_STYLES[run.status])}
          >
            {run.status === "waiting_approval" && (
              <span className="size-1.5 animate-pulse rounded-full bg-amber-500" />
            )}
            {RUN_STATUS_LABELS[run.status]}
          </Badge>
          {run.status === "running" && run.startedAt && <ElapsedSeconds startedAt={run.startedAt} />}
        </div>
        {isCancellable && (
          <button
            type="button"
            aria-label="Cancel run"
            title="Cancel run"
            onClick={handleCancel}
            disabled={cancel.isPending}
            className="rounded-md p-1 text-muted-foreground/60 transition-colors hover:text-destructive disabled:cursor-not-allowed"
          >
            <X className="size-3.5" />
          </button>
        )}
      </div>

      <p className="line-clamp-2 text-sm font-medium" title={run.task}>
        {run.task}
      </p>

      {run.status === "running" && <RunLoader trace={traceNames} />}

      {run.status === "running" && activity.length > 0 && (
        <div className="space-y-1 border-l border-border/70 pl-3">
          {activity.map((a, i) => (
            <div key={`${a.t}-${i}`} className="flex items-center gap-2 text-foreground/75">
              <Wrench className="size-3 shrink-0 text-muted-foreground" />
              <span className="truncate text-[11px] leading-tight">{humanizeActivity(a)}</span>
            </div>
          ))}
        </div>
      )}

      {run.status === "failed" && run.error && (
        <p className="text-xs text-destructive">{run.error}</p>
      )}

      {pending.length > 0 && (
        <ApprovalPanel pending={pending} isPending={resolve.isPending} onDecide={handleDecide} />
      )}

      <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border/40 pt-3">
        <p className="text-[11px] text-muted-foreground">
          {fmtTokens(run.inputTokens)} in / {fmtTokens(run.outputTokens)} out · $
          {run.costUsd.toFixed(5)} · {run.triggerSource}
        </p>
        <Button size="xs" variant="ghost" asChild className="text-muted-foreground">
          <Link to={`/chat/${run.conversationId}`}>
            <MessageSquare className="size-3.5" />
            Open conversation
          </Link>
        </Button>
      </div>
    </Card>
  );
}
