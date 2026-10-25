import { useState } from "react";
import { Link, useParams, useNavigate } from "react-router";
import { toast } from "sonner";
import {
  ArrowLeft,
  CalendarClock,
  FolderOpen,
  Loader2,
  MessageSquare,
  Pencil,
  Sidebar,
} from "lucide-react";
import { ApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useSidebar } from "@/components/layout/SidebarContext";
import { isRunActive, useBot, useBotRuns, useStartBotChat } from "@/features/bots/useBots";
import { BotFormDialog } from "@/features/bots/BotFormDialog";
import { TaskComposer } from "@/features/bots/TaskComposer";
import { RunCard, RUN_STATUS_LABELS, RUN_STATUS_STYLES } from "@/features/bots/RunCard";

type Tab = "runs" | "routines" | "workspace";

const TABS: Array<{ id: Tab; label: string }> = [
  { id: "runs", label: "Runs" },
  { id: "routines", label: "Routines" },
  { id: "workspace", label: "Workspace" },
];

// BotPage is the bot detail shell (header + placeholder sections); Tasks 9,
// 12 and 16 fill Runs, Routines and Workspace with real content.
export function BotPage() {
  const { botId } = useParams<{ botId: string }>();
  const navigate = useNavigate();
  const { toggleSidebar, setMobileOpen } = useSidebar();
  const { data, isLoading, isError } = useBot(botId ?? null);
  const startChat = useStartBotChat(botId ?? "");
  // Runs are polled here so the header chip and the Runs tab stay live; the
  // query stops refetching on its own once every run has settled.
  const runs = useBotRuns(botId ?? null);

  const [activeTab, setActiveTab] = useState<Tab>("runs");
  const [editOpen, setEditOpen] = useState(false);

  const bot = data?.bot;
  const latestRun = runs.data?.runs[0] ?? null;
  const runActive = runs.data?.runs.some((r) => isRunActive(r.status)) ?? false;

  async function handleChat() {
    try {
      const res = await startChat.mutateAsync();
      navigate(`/chat/${res.conversation.id}`);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Failed to start chat");
    }
  }

  if (isLoading) {
    return (
      <div className="flex h-full flex-col p-6 space-y-4">
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-full w-full rounded-2xl" />
      </div>
    );
  }

  if (isError || !bot) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-6">
        <p className="text-sm text-destructive">Bot not found.</p>
        <Button variant="outline" size="sm" asChild>
          <Link to="/bots">Back to Bots</Link>
        </Button>
      </div>
    );
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
            className="text-muted-foreground hover:text-foreground shrink-0"
          >
            <Sidebar className="size-4" />
          </Button>

          <Button
            variant="ghost"
            size="icon-xs"
            asChild
            className="text-muted-foreground hover:text-foreground shrink-0"
          >
            <Link to="/bots" title="Back to bots list">
              <ArrowLeft className="size-4" />
            </Link>
          </Button>

          <span className="flex size-7 shrink-0 items-center justify-center rounded-xl bg-muted text-base">
            {bot.emoji}
          </span>
          <div className="min-w-0">
            <h1 className="text-sm font-semibold tracking-tight truncate">{bot.name}</h1>
            <p className="line-clamp-1 text-xs text-muted-foreground" title={bot.instructions}>
              {bot.instructions || "No job set."}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          {latestRun ? (
            <Badge
              variant="outline"
              className={cn("gap-1.5 text-[10px]", RUN_STATUS_STYLES[latestRun.status])}
            >
              {latestRun.status === "waiting_approval" && (
                <span className="size-1.5 animate-pulse rounded-full bg-amber-500" />
              )}
              {RUN_STATUS_LABELS[latestRun.status]}
            </Badge>
          ) : (
            <Badge variant="outline" className="text-[10px] text-muted-foreground">
              Idle
            </Badge>
          )}
          <Button
            size="sm"
            variant="outline"
            onClick={() => setEditOpen(true)}
            className="gap-1.5"
          >
            <Pencil className="size-3.5" />
            <span className="hidden sm:inline">Settings</span>
          </Button>
          <Button size="sm" variant="brand" onClick={() => void handleChat()} disabled={startChat.isPending}>
            {startChat.isPending ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <MessageSquare className="size-3.5" />
            )}
            {startChat.isPending ? "Starting…" : "Chat"}
          </Button>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-5xl space-y-6 p-6">
          {/* Tabs */}
          <div className="flex items-center gap-2 border-b border-border text-xs">
            {TABS.map((tab) => (
              <button
                key={tab.id}
                type="button"
                onClick={() => setActiveTab(tab.id)}
                className={cn(
                  "flex items-center gap-2 border-b-2 px-3 pb-2.5 font-semibold transition-colors",
                  activeTab === tab.id
                    ? "border-primary text-foreground"
                    : "border-transparent text-muted-foreground hover:text-foreground"
                )}
              >
                <span>{tab.label}</span>
              </button>
            ))}
          </div>

          {activeTab === "runs" && (
            <div className="space-y-4">
              <TaskComposer bot={bot} runActive={runActive} />

              {runs.isLoading && <Skeleton className="h-44 rounded-2xl" />}

              {!runs.isLoading && runs.data && runs.data.runs.length > 0 && (
                <div className="space-y-3">
                  {runs.data.runs.map((r) => (
                    <RunCard key={r.id} run={r} />
                  ))}
                </div>
              )}

              {!runs.isLoading && runs.data && runs.data.runs.length === 0 && (
                <div className="flex flex-col items-center justify-center gap-3 rounded-2xl border border-dashed border-border/80 p-12 text-center">
                  <div className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
                    <CalendarClock className="size-6" />
                  </div>
                  <div className="space-y-1">
                    <h3 className="text-sm font-semibold">No runs yet</h3>
                    <p className="max-w-sm text-xs text-muted-foreground">
                      Scheduled and background runs will appear here. Give {bot.name} a task above
                      to get started.
                    </p>
                  </div>
                </div>
              )}
            </div>
          )}

          {activeTab === "routines" && (
            <div className="flex flex-col items-center justify-center gap-3 rounded-2xl border border-dashed border-border/80 p-12 text-center">
              <div className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
                <CalendarClock className="size-6" />
              </div>
              <div className="space-y-1">
                <h3 className="text-sm font-semibold">No routines yet</h3>
                <p className="max-w-sm text-xs text-muted-foreground">
                  Routines let {bot.name} work on a schedule — daily digests, weekly reports, and
                  more.
                </p>
              </div>
            </div>
          )}

          {activeTab === "workspace" && (
            <div className="flex flex-col items-center justify-center gap-3 rounded-2xl border border-dashed border-border/80 p-12 text-center">
              <div className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
                <FolderOpen className="size-6" />
              </div>
              <div className="space-y-1">
                <h3 className="text-sm font-semibold">Nothing in the workspace yet</h3>
                <p className="max-w-sm text-xs text-muted-foreground">
                  Files and artifacts {bot.name} produces will land here.
                </p>
              </div>
            </div>
          )}
        </div>
      </div>

      <BotFormDialog open={editOpen} onOpenChange={setEditOpen} bot={bot} />
    </div>
  );
}
