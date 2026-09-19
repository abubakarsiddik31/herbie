import { useState } from "react";
import { Gauge, CheckCircle2, AlertTriangle, Sparkles, Layers } from "lucide-react";
import { Tooltip as TooltipPrimitive } from "radix-ui";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";

export interface ContextStatusMeterProps {
  estimatedTokens: number;
  thresholdTokens?: number;
  keepRecent?: number;
  compacted?: boolean;
  className?: string;
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) {
    const v = n / 1_000_000;
    return v % 1 === 0 ? `${v.toFixed(0)}M` : `${v.toFixed(1)}M`;
  }
  if (n >= 1_000) {
    const v = n / 1_000;
    return v % 1 === 0 ? `${v.toFixed(0)}k` : `${v.toFixed(1)}k`;
  }
  return n.toLocaleString();
}

export function ContextStatusMeter({
  estimatedTokens,
  thresholdTokens = 40_000,
  keepRecent = 10,
  compacted = false,
  className,
}: ContextStatusMeterProps) {
  const [open, setOpen] = useState(false);
  const threshold = thresholdTokens > 0 ? thresholdTokens : 40_000;
  const pct = Math.min(100, Math.max(0, Math.round((estimatedTokens / threshold) * 100)));

  // Color levels based on context consumption
  const isHigh = pct >= 90;
  const isWarning = pct >= 70 && pct < 90;

  let pillClasses = "text-muted-foreground hover:text-foreground bg-muted/30 hover:bg-muted/60 border-border/40";
  if (compacted) {
    pillClasses = "text-primary border-primary/30 bg-primary/10 hover:bg-primary/20";
  } else if (isHigh) {
    pillClasses = "text-rose-500 border-rose-500/30 bg-rose-500/10 hover:bg-rose-500/20 font-medium";
  } else if (isWarning) {
    pillClasses = "text-amber-500 border-amber-500/30 bg-amber-500/10 hover:bg-amber-500/20";
  }

  let statusLabel = "Healthy Context";
  let statusBadgeVariant: "secondary" | "destructive" = "secondary";
  if (compacted) {
    statusLabel = "Context Compacted";
  } else if (isHigh) {
    statusLabel = "Compaction Imminent";
    statusBadgeVariant = "destructive" as const;
  } else if (isWarning) {
    statusLabel = "Approaching Threshold";
  }

  return (
    <TooltipPrimitive.Provider delayDuration={150}>
      <TooltipPrimitive.Root open={open} onOpenChange={setOpen}>
        <TooltipPrimitive.Trigger asChild>
          <button
            type="button"
            aria-label={`Context status: ${formatTokens(estimatedTokens)} of ${formatTokens(threshold)} tokens (${pct}%)`}
            onClick={() => setOpen((prev) => !prev)}
            className={cn(
              "inline-flex h-7 items-center gap-1.5 rounded-lg border px-2 text-[11px] font-mono transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
              pillClasses,
              className
            )}
          >
            {compacted ? (
              <Sparkles className="size-3 text-primary animate-pulse" />
            ) : isHigh ? (
              <AlertTriangle className="size-3 text-rose-500" />
            ) : (
              <Gauge className="size-3 text-muted-foreground" />
            )}

            <span>
              {formatTokens(estimatedTokens)} / {formatTokens(threshold)}
            </span>

            {compacted && (
              <span className="hidden xs:inline text-[10px] font-sans font-medium px-1 rounded bg-primary/20 text-primary">
                compacted
              </span>
            )}
          </button>
        </TooltipPrimitive.Trigger>

        <TooltipPrimitive.Portal>
          <TooltipPrimitive.Content
            side="top"
            sideOffset={8}
            collisionPadding={12}
            className="z-50 w-72 rounded-xl border bg-popover p-3 text-popover-foreground shadow-xl animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95"
          >
            {/* Header */}
            <div className="flex items-center justify-between gap-2 border-b border-border/40 pb-2">
              <div className="flex items-center gap-1.5 text-xs font-semibold">
                <Layers className="size-3.5 text-primary" />
                <span>Context & Compaction</span>
              </div>
              <Badge variant={statusBadgeVariant} className="text-[10px] h-5 px-1.5 font-normal">
                {statusLabel}
              </Badge>
            </div>

            {/* Meter Bar */}
            <div className="mt-2.5 space-y-1.5">
              <div className="flex justify-between text-[11px] font-mono text-muted-foreground">
                <span>Usage</span>
                <span className="font-semibold text-foreground">
                  {estimatedTokens.toLocaleString()} / {threshold.toLocaleString()} tokens ({pct}%)
                </span>
              </div>
              <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
                <div
                  className={cn(
                    "h-full transition-all duration-300 rounded-full",
                    compacted ? "bg-primary" : isHigh ? "bg-rose-500" : isWarning ? "bg-amber-500" : "bg-emerald-500"
                  )}
                  style={{ width: `${Math.max(4, pct)}%` }}
                />
              </div>
            </div>

            {/* Compaction Details */}
            <div className="mt-3 space-y-1.5 text-[11px] text-muted-foreground leading-relaxed">
              <div className="flex items-start gap-1.5">
                <CheckCircle2 className="size-3.5 shrink-0 text-primary mt-0.5" />
                <p>
                  <strong className="text-foreground font-medium">Trigger Threshold: </strong>
                  Auto-compacts when conversation exceeds{" "}
                  <span className="font-mono text-foreground">{threshold.toLocaleString()}</span> tokens.
                </p>
              </div>
              <div className="flex items-start gap-1.5">
                <CheckCircle2 className="size-3.5 shrink-0 text-primary mt-0.5" />
                <p>
                  <strong className="text-foreground font-medium">Preserved Window: </strong>
                  Keeps the <span className="font-mono text-foreground">{keepRecent}</span> most recent messages
                  verbatim; older turns are summarized into compact background memory.
                </p>
              </div>

              {compacted && (
                <div className="mt-2 rounded-md bg-primary/10 p-2 text-primary border border-primary/20 text-[10px]">
                  ✓ Older context has been compacted into a concise summary to preserve performance.
                </div>
              )}

              <div className="pt-2 border-t border-border/30 text-[10px] text-muted-foreground/80">
                <span className="font-semibold text-foreground/90">Note: </span>
                This meter tracks active thread memory. Multi-step agent turns may accumulate higher billed input tokens across iterative round trips.
              </div>
            </div>

            <TooltipPrimitive.Arrow className="fill-popover" width={10} height={5} />
          </TooltipPrimitive.Content>
        </TooltipPrimitive.Portal>
      </TooltipPrimitive.Root>
    </TooltipPrimitive.Provider>
  );
}
