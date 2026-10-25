import { useState } from "react";
import { toast } from "sonner";
import { Loader2, Send } from "lucide-react";
import type { Bot } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { useEnqueueTask } from "@/features/bots/useBots";

interface Props {
  bot: Bot;
  /** True while the bot already has a queued/running/waiting_approval run. */
  runActive: boolean;
}

// TaskComposer is the entry point for background work on the bot's Runs
// tab: type a task, hit Run, and the run executes even with the tab closed.
export function TaskComposer({ bot, runActive }: Props) {
  const enqueue = useEnqueueTask(bot.id);
  const [task, setTask] = useState("");

  function submit() {
    const trimmed = task.trim();
    if (!trimmed || runActive || enqueue.isPending) return;
    enqueue.mutate(
      { task: trimmed },
      {
        onSuccess: () => {
          setTask("");
          toast.success("Task queued");
        },
      },
    );
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    submit();
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      submit();
    }
  }

  return (
    <form onSubmit={handleSubmit} className="rounded-2xl border border-border/80 bg-muted/20 p-3">
      <textarea
        value={task}
        onChange={(e) => setTask(e.target.value)}
        onKeyDown={handleKeyDown}
        rows={2}
        disabled={runActive}
        aria-label={`Task for ${bot.name}`}
        placeholder={`Give ${bot.name} a task…`}
        className="min-h-16 w-full resize-y rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-60"
      />
      <div className="mt-2 flex items-center justify-between gap-2">
        <p className="text-[11px] text-muted-foreground">
          {runActive
            ? "One task at a time — cancel the active run to start another."
            : "Runs in the background — you can close this tab."}
        </p>
        <Button
          type="submit"
          size="sm"
          variant="brand"
          disabled={!task.trim() || runActive || enqueue.isPending}
        >
          {enqueue.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <Send className="size-3.5" />
          )}
          {enqueue.isPending ? "Starting…" : "Run task"}
        </Button>
      </div>
    </form>
  );
}
