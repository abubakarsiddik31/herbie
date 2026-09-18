import { memo } from "react";
import { Handle, Position, type NodeProps } from "@xyflow/react";
import { CheckCircle2, Loader2, XCircle } from "lucide-react";
import { cn } from "@/lib/utils";
import type { NodeExecutionResult } from "@/lib/types";
import { getNodeDefinition } from "./nodeTypes";

export interface CustomNodeData {
  name: string;
  type: string;
  config: Record<string, unknown>;
  executionResult?: NodeExecutionResult;
  [key: string]: unknown;
}

export const WorkflowNodeComponent = memo(function WorkflowNodeComponent({
  id: _id,
  data,
  selected,
}: NodeProps) {
  const nodeData = data as unknown as CustomNodeData;
  const def = getNodeDefinition(nodeData.type);
  const Icon = def.icon;
  const isCondition = nodeData.type === "condition";
  const isTrigger = def.category === "trigger";
  const isOutput = def.category === "output";

  const result = nodeData.executionResult;
  const isRunning = result?.status === "running";
  const isSuccess = result?.status === "success";
  const isFailed = result?.status === "failed";

  return (
    <div
      className={cn(
        "group relative min-w-[220px] max-w-[280px] rounded-xl border bg-card p-3 shadow-sm transition-all duration-150 hover:shadow-md",
        selected ? "border-primary ring-2 ring-primary/20 shadow-md" : "border-border",
        isRunning && "ring-2 ring-blue-500/40 border-blue-500/80 animate-pulse",
        isSuccess && "border-emerald-500/50",
        isFailed && "border-destructive ring-1 ring-destructive/30"
      )}
    >
      {/* Target handle (input) - non-triggers only */}
      {!isTrigger && (
        <Handle
          type="target"
          position={Position.Left}
          className="size-3 rounded-full border-2 border-background bg-muted-foreground/60 transition-colors hover:bg-primary"
        />
      )}

      {/* Header with Icon and Name */}
      <div className="flex items-center gap-2.5">
        <div className={cn("flex size-8 shrink-0 items-center justify-center rounded-lg border", def.bgColor)}>
          <Icon className={cn("size-4", def.color)} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-xs font-semibold text-foreground tracking-tight">
            {nodeData.name || def.label}
          </p>
          <p className="truncate text-[10px] text-muted-foreground uppercase font-mono tracking-wider">
            {def.label}
          </p>
        </div>

        {/* Execution status indicator */}
        {isRunning && (
          <Loader2 className="size-3.5 animate-spin text-blue-500 shrink-0" />
        )}
        {isSuccess && (
          <div className="flex items-center gap-1 shrink-0 text-emerald-500">
            <CheckCircle2 className="size-3.5" />
            {result.durationMs !== undefined && (
              <span className="font-mono text-[9px] text-muted-foreground">
                {result.durationMs}ms
              </span>
            )}
          </div>
        )}
        {isFailed && (
          <XCircle className="size-3.5 text-destructive shrink-0" />
        )}
      </div>

      {/* Condition Handles */}
      {isCondition ? (
        <div className="mt-3 flex flex-col gap-2 pt-2 border-t border-border/60">
          <div className="relative flex items-center justify-between text-[10px]">
            <span className="font-medium text-emerald-600 dark:text-emerald-400">True</span>
            <Handle
              type="source"
              position={Position.Right}
              id="true"
              className="!top-1.5 size-3 rounded-full border-2 border-background bg-emerald-500"
            />
          </div>
          <div className="relative flex items-center justify-between text-[10px]">
            <span className="font-medium text-destructive">False</span>
            <Handle
              type="source"
              position={Position.Right}
              id="false"
              className="!top-6 size-3 rounded-full border-2 border-background bg-destructive"
            />
          </div>
        </div>
      ) : (
        /* Standard Source Handle (output) - non-outputs only */
        !isOutput && (
          <Handle
            type="source"
            position={Position.Right}
            className="size-3 rounded-full border-2 border-background bg-muted-foreground/60 transition-colors hover:bg-primary"
          />
        )
      )}
    </div>
  );
});
