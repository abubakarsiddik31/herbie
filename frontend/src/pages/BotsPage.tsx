import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { toast } from "sonner";
import {
  Bot as BotIcon,
  Loader2,
  MessageSquare,
  Pencil,
  Plus,
  Sidebar,
  Trash2,
} from "lucide-react";
import { ApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { Bot } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { useSidebar } from "@/components/layout/SidebarContext";
import { BotFormDialog } from "@/features/bots/BotFormDialog";
import { useBots, useDeleteBot, useStartBotChat } from "@/features/bots/useBots";

/** Avatar tint per bot color; keys mirror the backend color whitelist. */
const BOT_AVATAR_CLASS: Record<string, string> = {
  sage: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
  sky: "bg-sky-500/10 text-sky-700 dark:text-sky-400",
  amber: "bg-amber-500/10 text-amber-700 dark:text-amber-400",
  rose: "bg-rose-500/10 text-rose-700 dark:text-rose-400",
  violet: "bg-violet-500/10 text-violet-700 dark:text-violet-400",
  slate: "bg-slate-500/10 text-slate-700 dark:text-slate-400",
};

function avatarClass(color: string) {
  return BOT_AVATAR_CLASS[color] ?? BOT_AVATAR_CLASS.sage;
}

function BotCard({ bot, onEdit, onDelete }: { bot: Bot; onEdit: (bot: Bot) => void; onDelete: (bot: Bot) => void }) {
  const navigate = useNavigate();
  const startChat = useStartBotChat(bot.id);

  async function handleChat() {
    try {
      const res = await startChat.mutateAsync();
      navigate(`/chat/${res.conversation.id}`);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Failed to start chat");
    }
  }

  return (
    <Card className="group gap-3 p-5 py-5 shadow-xs transition-all hover:border-foreground/25 hover:shadow-sm">
      <div className="flex items-start justify-between gap-2">
        <Link to={`/bots/${bot.id}`} className="flex min-w-0 flex-1 items-center gap-3 outline-none">
          <span
            className={cn(
              "flex size-10 shrink-0 items-center justify-center rounded-2xl text-xl",
              avatarClass(bot.color)
            )}
          >
            {bot.emoji}
          </span>
          <div className="min-w-0">
            <h3 className="truncate text-sm font-semibold">{bot.name}</h3>
            <p className="line-clamp-1 text-xs text-muted-foreground" title={bot.instructions}>
              {bot.instructions || "No job set."}
            </p>
          </div>
        </Link>
        {/* Status chip intentionally stays "Idle" here: the bots list endpoint
         *  carries no latest-run data and per-bot run polling on this page was
         *  ruled out (see task-9 report); revisit during T12/morning polish. */}
        <Badge variant="outline" className="shrink-0 text-[10px] text-muted-foreground">
          Idle
        </Badge>
      </div>

      <div className="mt-auto flex items-center gap-1.5 border-t border-border/40 pt-3">
        <Button size="xs" onClick={() => void handleChat()} disabled={startChat.isPending}>
          {startChat.isPending ? (
            <Loader2 className="animate-spin" />
          ) : (
            <MessageSquare />
          )}
          {startChat.isPending ? "Starting…" : "Chat"}
        </Button>
        <Button size="xs" variant="ghost" onClick={() => onEdit(bot)}>
          <Pencil />
          Settings
        </Button>
        <Button
          size="xs"
          variant="ghost"
          aria-label={`Delete ${bot.name}`}
          title={`Delete ${bot.name}`}
          onClick={() => onDelete(bot)}
          className="ml-auto text-muted-foreground hover:text-destructive"
        >
          <Trash2 />
        </Button>
      </div>
    </Card>
  );
}

export function BotsPage() {
  const { toggleSidebar, setMobileOpen } = useSidebar();
  const { data, isLoading, isError, refetch } = useBots();
  const bots = data?.bots;
  const count = data?.count ?? 0;

  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Bot | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Bot | null>(null);
  const deleteBot = useDeleteBot(pendingDelete?.id ?? "");

  function handleDelete() {
    if (!pendingDelete) return;
    deleteBot.mutate(undefined, {
      onSuccess: () => {
        setPendingDelete(null);
        toast.success("Bot deleted");
      },
      onError: (err) =>
        toast.error(err instanceof ApiError ? err.message : "Failed to delete bot"),
    });
  }

  return (
    <div className="flex h-full flex-col bg-background">
      <header className="flex items-center justify-between gap-2 border-b border-border/60 px-4 py-2 bg-background/80 backdrop-blur-xs z-10 shrink-0">
        <div className="flex items-center gap-2 min-w-0">
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
            <BotIcon className="size-4 text-muted-foreground" />
            <h1 className="text-sm font-semibold tracking-tight">Bots</h1>
            {data && (
              <span className="rounded-full bg-muted px-2 py-0.5 font-mono text-[10px] text-muted-foreground">
                {count}
              </span>
            )}
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="brand"
            onClick={() => {
              setEditing(null);
              setFormOpen(true);
            }}
          >
            <Plus className="size-3.5" /> New bot
          </Button>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-5xl space-y-6 p-6">
          <div className="flex flex-col gap-1">
            <h2 className="text-lg font-semibold tracking-tight">Bots — always-on teammates</h2>
            <p className="text-xs text-muted-foreground">
              Give a teammate a name, a job, and a model. Bots keep their memory and keep working
              after you close the tab.
            </p>
          </div>

          {isLoading && (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-40 rounded-2xl" />
              ))}
            </div>
          )}

          {isError && (
            <div className="flex flex-col items-center gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 p-8 text-center">
              <p className="text-xs text-destructive">Failed to load bots.</p>
              <Button variant="outline" size="sm" onClick={() => void refetch()}>
                Retry
              </Button>
            </div>
          )}

          {!isLoading && !isError && (!bots || bots.length === 0) && (
            <div className="flex flex-col items-center justify-center gap-4 rounded-2xl border border-dashed border-border/80 p-12 text-center">
              <div className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
                <BotIcon className="size-6" />
              </div>
              <div className="space-y-1">
                <h3 className="text-sm font-semibold">No bots yet</h3>
                <p className="max-w-sm text-xs text-muted-foreground">
                  A bot is a teammate with a name, a job, and a model — it keeps its memory and
                  keeps working after you close the tab.
                </p>
              </div>
              <Button
                size="sm"
                variant="brand"
                onClick={() => {
                  setEditing(null);
                  setFormOpen(true);
                }}
              >
                <Plus className="size-3.5" /> Create your first bot
              </Button>
            </div>
          )}

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {bots?.map((b) => (
              <BotCard
                key={b.id}
                bot={b}
                onEdit={(bot) => {
                  setEditing(bot);
                  setFormOpen(true);
                }}
                onDelete={setPendingDelete}
              />
            ))}
          </div>
        </div>
      </div>

      <BotFormDialog
        open={formOpen}
        onOpenChange={(open) => {
          setFormOpen(open);
          if (!open) setEditing(null);
        }}
        bot={editing ?? undefined}
      />

      {/* Delete Confirmation Dialog */}
      <Dialog open={pendingDelete !== null} onOpenChange={(open) => !open && setPendingDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete bot?</DialogTitle>
            <DialogDescription>
              “{pendingDelete?.name}” and its memories will be permanently removed.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingDelete(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleteBot.isPending}>
              {deleteBot.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
