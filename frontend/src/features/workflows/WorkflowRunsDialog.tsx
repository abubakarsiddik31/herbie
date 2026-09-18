import { useState } from "react";
import { CheckCircle2, Clock, History, Loader2, XCircle } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useWorkflowRuns } from "./useWorkflows";
import { PayloadViewer } from "./PayloadViewer";
import type { WorkflowRun } from "@/lib/types";

interface WorkflowRunsDialogProps {
  workflowId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function WorkflowRunsDialog({ workflowId, open, onOpenChange }: WorkflowRunsDialogProps) {
  const { data: runs, isLoading } = useWorkflowRuns(workflowId);
  const [selectedRun, setSelectedRun] = useState<WorkflowRun | null>(null);

  // Auto-select latest run if current selection is not available
  const activeRun = selectedRun ?? (runs && runs.length > 0 ? runs[0] : null);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-4xl lg:max-w-5xl max-h-[88vh] w-full flex flex-col p-0 overflow-hidden">
        <DialogHeader className="p-4 border-b border-border">
          <DialogTitle className="flex items-center gap-2 text-base">
            <History className="size-4 text-primary" />
            <span>Execution History</span>
          </DialogTitle>
          <DialogDescription className="text-xs">
            Review past workflow executions, payload inputs, outputs, and node traces.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-1 min-h-0 min-w-0 divide-x divide-border">
          {/* Left: Runs list */}
          <div className="w-60 sm:w-64 md:w-72 shrink-0 flex flex-col min-h-0">
            <ScrollArea className="flex-1 p-3">
              {isLoading && <p className="text-xs text-muted-foreground p-2">Loading past runs...</p>}
              {!isLoading && (!runs || runs.length === 0) && (
                <p className="text-xs text-muted-foreground p-4 text-center">No runs recorded yet.</p>
              )}
              <div className="space-y-1.5">
                {runs?.map((r) => {
                  const isSelected = activeRun?.id === r.id;
                  return (
                    <button
                      key={r.id}
                      type="button"
                      onClick={() => setSelectedRun(r)}
                      className={`w-full text-left rounded-lg p-2.5 text-xs transition-colors border ${
                        isSelected
                          ? "bg-accent text-accent-foreground border-border font-medium shadow-xs"
                          : "border-transparent hover:bg-muted/50 text-muted-foreground hover:text-foreground"
                      }`}
                    >
                      <div className="flex items-center justify-between mb-1">
                        <span className="flex items-center gap-1.5 font-semibold text-foreground">
                          {r.status === "success" && <CheckCircle2 className="size-3.5 text-emerald-500" />}
                          {r.status === "failed" && <XCircle className="size-3.5 text-destructive" />}
                          {r.status === "running" && <Loader2 className="size-3.5 animate-spin text-blue-500" />}
                          <span className="capitalize">{r.status}</span>
                        </span>
                        <span className="text-[10px] font-mono text-muted-foreground">{r.durationMs}ms</span>
                      </div>
                      <div className="flex items-center justify-between text-[11px] text-muted-foreground">
                        <span className="capitalize">via {r.triggerSource}</span>
                        <span>{new Date(r.createdAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</span>
                      </div>
                    </button>
                  );
                })}
              </div>
            </ScrollArea>
          </div>

          {/* Right: Selected Run Details */}
          <div className="flex-1 flex flex-col min-h-0 min-w-0 bg-muted/20">
            <ScrollArea className="flex-1 min-w-0">
              <div className="p-4 space-y-4 text-xs min-w-0">
                {!activeRun ? (
                  <div className="flex flex-col items-center justify-center h-48 text-muted-foreground text-xs">
                    <Clock className="size-8 mb-2 opacity-30" />
                    <p>Select an execution on the left to inspect its payloads and node breakdown.</p>
                  </div>
                ) : (
                  <>
                    <div className="flex items-center justify-between rounded-lg border border-border bg-card p-3">
                      <div>
                        <p className="font-semibold text-foreground">Run #{activeRun.id.slice(0, 8)}</p>
                        <p className="text-[11px] text-muted-foreground mt-0.5">
                          Started: {new Date(activeRun.createdAt).toLocaleString()}
                        </p>
                      </div>
                      <div className="text-right">
                        <span
                          className={`inline-block px-2 py-0.5 rounded-full text-[11px] font-semibold uppercase ${
                            activeRun.status === "success"
                              ? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
                              : "bg-destructive/15 text-destructive"
                          }`}
                        >
                          {activeRun.status}
                        </span>
                        <p className="text-[11px] font-mono text-muted-foreground mt-0.5">{activeRun.durationMs} ms</p>
                      </div>
                    </div>

                    {activeRun.error && (
                      <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-destructive">
                        <span className="font-semibold">Error Message:</span> {activeRun.error}
                      </div>
                    )}

                    {/* Output Data */}
                    <PayloadViewer
                      title="Final Output Data"
                      data={activeRun.outputData}
                      defaultMode="rich"
                    />

                    {/* Input Data */}
                    <PayloadViewer
                      title="Trigger Input Payload"
                      data={activeRun.inputData}
                      defaultMode="rich"
                    />

                    {/* Step Results breakdown */}
                    {activeRun.nodeResults && Object.keys(activeRun.nodeResults).length > 0 && (
                      <div className="space-y-2 pt-2 border-t border-border">
                        <p className="font-semibold text-foreground text-xs">Steps Breakdown</p>
                        <div className="space-y-1.5">
                          {Object.values(activeRun.nodeResults).map((step) => (
                            <div
                              key={step.nodeId}
                              className="flex items-center justify-between rounded-lg border border-border bg-card px-3 py-2 text-xs"
                            >
                              <div className="flex items-center gap-2 min-w-0">
                                {step.status === "success" ? (
                                  <CheckCircle2 className="size-3.5 shrink-0 text-emerald-500" />
                                ) : (
                                  <XCircle className="size-3.5 shrink-0 text-destructive" />
                                )}
                                <span className="font-medium text-foreground truncate">{step.nodeName || step.nodeId}</span>
                                <span className="text-[10px] text-muted-foreground font-mono truncate">({step.nodeType})</span>
                              </div>
                              <span className="text-[10px] font-mono text-muted-foreground shrink-0 ml-2">{step.durationMs}ms</span>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}
                  </>
                )}
              </div>
            </ScrollArea>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
