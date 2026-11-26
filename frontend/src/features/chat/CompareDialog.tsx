import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ApiError } from "@/lib/api";
import type { ModelsResponse } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useActivateBranch } from "./useConversations";
import { useCompare } from "./useCompare";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  conversationId?: string;
  models?: ModelsResponse;
  prompt: string;
  onRan?: () => void;
}

export function CompareDialog({ open, onOpenChange, conversationId, models, prompt, onRan }: Props) {
  const compare = useCompare(conversationId);
  const activate = useActivateBranch(conversationId ?? null);
  const [selected, setSelected] = useState<string[]>([]);

  const choices = (models?.models ?? []).filter((m) => m.id !== "auto");

  useEffect(() => {
    if (!open) return;
    compare.reset();
    setSelected(choices.slice(0, 2).map((m) => m.id));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  function toggle(id: string) {
    setSelected((current) => {
      if (current.includes(id)) return current.filter((x) => x !== id);
      if (current.length >= 4) return current;
      return [...current, id];
    });
  }

  const pending = compare.isPending || activate.isPending;
  const results = compare.data?.results;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Compare models</DialogTitle>
          <DialogDescription>
            Run this prompt against 2–4 models, then pick the answer to keep.
          </DialogDescription>
        </DialogHeader>

        {!results ? (
          <div className="space-y-3">
            <p className="line-clamp-3 rounded-xl border border-border/60 bg-muted/20 p-3 text-xs text-muted-foreground">
              {prompt || "Type a prompt first."}
            </p>
            <div className="grid gap-1.5 sm:grid-cols-2">
              {choices.map((m) => (
                <label
                  key={m.id}
                  className="flex cursor-pointer items-center gap-2 rounded-lg border border-border/50 px-2.5 py-1.5 text-xs"
                >
                  <input
                    type="checkbox"
                    checked={selected.includes(m.id)}
                    onChange={() => toggle(m.id)}
                    disabled={pending}
                  />
                  <span className="truncate">{m.label}</span>
                </label>
              ))}
            </div>
            {selected.length < 2 && (
              <p className="text-[11px] text-muted-foreground">Choose at least two models.</p>
            )}
          </div>
        ) : (
          <div className="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
            {results.map((result) => {
              const label =
                choices.find((m) => m.id === result.model)?.label ?? result.model;
              return (
                <div key={result.model} className="rounded-xl border border-border/60 p-3">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-xs font-semibold">{label}</span>
                    <span className="font-mono text-[10px] text-muted-foreground">
                      {result.inputTokens + result.outputTokens} tok · ${result.costUsd.toFixed(5)}
                    </span>
                  </div>
                  {result.error ? (
                    <p className="mt-1 text-xs text-destructive">{result.error}</p>
                  ) : (
                    <pre className="mt-2 max-h-64 overflow-y-auto whitespace-pre-wrap text-xs text-foreground/90">
                      {result.content}
                    </pre>
                  )}
                  {result.messageId && (
                    <div className="mt-2 flex justify-end">
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={pending}
                        onClick={() =>
                          activate.mutate(result.messageId!, {
                            onSuccess: () => {
                              toast.success("Answer selected");
                              onOpenChange(false);
                            },
                            onError: (err) =>
                              toast.error(
                                err instanceof ApiError ? err.message : "Could not select answer",
                              ),
                          })
                        }
                      >
                        Pick this answer
                      </Button>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}

        <DialogFooter className="gap-2 sm:justify-end">
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)} disabled={pending}>
            {results ? "Close" : "Cancel"}
          </Button>
          {!results && (
            <Button
              variant="brand"
              size="sm"
              disabled={pending || selected.length < 2 || selected.length > 4 || !prompt.trim()}
              onClick={() =>
                compare.mutate(
                  { content: prompt, models: selected },
                  {
                    onSuccess: () => {
                      onRan?.();
                      toast.success("Compare finished");
                    },
                    onError: (err) =>
                      toast.error(err instanceof ApiError ? err.message : "Compare failed"),
                  },
                )
              }
            >
              {compare.isPending ? "Comparing…" : `Compare ${selected.length} models`}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
