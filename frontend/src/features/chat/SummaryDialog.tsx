import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  useClearSummary,
  useCompactNow,
  useConversationSummary,
  useSaveSummary,
} from "./useSummary";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  conversationId?: string;
}

export function SummaryDialog({ open, onOpenChange, conversationId }: Props) {
  const { data, isLoading } = useConversationSummary(open ? conversationId : undefined);
  const save = useSaveSummary(conversationId);
  const clear = useClearSummary(conversationId);
  const compact = useCompactNow(conversationId);
  const [draft, setDraft] = useState("");

  useEffect(() => {
    if (open && data) setDraft(data.summary);
  }, [open, data]);

  const pending = save.isPending || clear.isPending || compact.isPending;
  const errorMessage = (err: unknown, fallback: string) =>
    err instanceof ApiError ? err.message : fallback;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>Conversation summary</DialogTitle>
          <DialogDescription>
            Older turns can be replaced by this summary in future runs. Edit it, or summarize now.
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : (
          <div className="space-y-3">
            <div className="flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted-foreground">
              <span>
                Covered: <span className="font-mono text-foreground">{data?.coveredMessages ?? 0}</span>{" "}
                / {data?.totalMessages ?? 0} messages
              </span>
              <span>
                Auto-compacts at{" "}
                <span className="font-mono text-foreground">
                  {(data?.thresholdTokens ?? 40000).toLocaleString()}
                </span>{" "}
                tokens · keeps {data?.keepRecent ?? 10}
              </span>
            </div>
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              maxLength={8000}
              rows={10}
              placeholder="No summary yet. Use “Summarize now” to generate one."
              className="w-full resize-y rounded-xl border border-border/60 bg-background/60 p-3 text-xs outline-none focus-visible:ring-1 focus-visible:ring-ring"
            />
            {draft === "" && (data?.coveredMessages ?? 0) > 0 && (
              <p className="text-[11px] text-muted-foreground">
                An empty summary clears the applied coverage on save.
              </p>
            )}
          </div>
        )}

        <DialogFooter className="gap-2 sm:justify-between">
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={pending || !conversationId}
              onClick={() =>
                compact.mutate(undefined, {
                  onSuccess: (res) => {
                    setDraft(res.summary);
                    toast.success(`Summarized ${res.coveredMessages} messages`);
                  },
                  onError: (err) => toast.error(errorMessage(err, "Could not summarize")),
                })
              }
            >
              {compact.isPending ? "Summarizing…" : "Summarize now"}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={pending || !data?.summary}
              onClick={() =>
                clear.mutate(undefined, {
                  onSuccess: () => {
                    setDraft("");
                    toast.success("Summary cleared");
                  },
                  onError: (err) => toast.error(errorMessage(err, "Could not clear summary")),
                })
              }
            >
              Clear
            </Button>
          </div>
          <div className="flex gap-2">
            <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)} disabled={pending}>
              Cancel
            </Button>
            <Button
              variant="brand"
              size="sm"
              disabled={pending || !conversationId}
              onClick={() =>
                save.mutate(draft, {
                  onSuccess: () => toast.success("Summary saved"),
                  onError: (err) => toast.error(errorMessage(err, "Could not save summary")),
                })
              }
            >
              {save.isPending ? "Saving…" : "Save"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
