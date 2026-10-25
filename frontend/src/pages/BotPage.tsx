import { useState } from "react";
import { Link, useParams, useNavigate } from "react-router";
import { toast } from "sonner";
import {
  ArrowLeft,
  Brain,
  CalendarClock,
  FolderOpen,
  Loader2,
  MessageSquare,
  Pencil,
  Play,
  Plus,
  Sidebar,
  Trash2,
} from "lucide-react";
import { ApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { BotFileMeta, BotRoutine } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { useSidebar } from "@/components/layout/SidebarContext";
import {
  isRunActive,
  useBot,
  useBotFiles,
  useBotMemories,
  useBotRoutines,
  useBotRuns,
  useDeleteBotMemory,
  useDeleteRoutine,
  useRunRoutineNow,
  useStartBotChat,
  useUpdateRoutine,
} from "@/features/bots/useBots";
import { BotFormDialog } from "@/features/bots/BotFormDialog";
import { FileViewerDialog, formatBytes } from "@/features/bots/FileViewerDialog";
import { RoutineFormDialog } from "@/features/bots/RoutineFormDialog";
import { TaskComposer } from "@/features/bots/TaskComposer";
import { RunCard, RUN_STATUS_LABELS, RUN_STATUS_STYLES } from "@/features/bots/RunCard";

type Tab = "runs" | "routines" | "workspace";

const TABS: Array<{ id: Tab; label: string }> = [
  { id: "runs", label: "Runs" },
  { id: "routines", label: "Routines" },
  { id: "workspace", label: "Workspace" },
];

// RoutinesPanel lists the bot's cron routines with enable toggles, manual
// runs and deletes. The runs they produce are shown in the Runs tab.
function RoutinesPanel({ botId, botName }: { botId: string; botName: string }) {
  const routines = useBotRoutines(botId);
  const updateRoutine = useUpdateRoutine(botId);
  const deleteRoutine = useDeleteRoutine(botId);
  const runNow = useRunRoutineNow(botId);

  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<BotRoutine | null>(null);
  const [pendingDelete, setPendingDelete] = useState<BotRoutine | null>(null);

  function handleToggle(routine: BotRoutine, enabled: boolean) {
    updateRoutine.mutate(
      { routineId: routine.id, enabled },
      {
        onError: (err) =>
          toast.error(err instanceof ApiError ? err.message : "Failed to update routine"),
      }
    );
  }

  function handleRunNow(routine: BotRoutine) {
    runNow.mutate(routine.id, {
      onSuccess: () => toast.success(`Queued "${routine.name}"`),
      onError: (err) =>
        toast.error(err instanceof ApiError ? err.message : "Failed to queue run"),
    });
  }

  function handleDelete() {
    if (!pendingDelete) return;
    deleteRoutine.mutate(pendingDelete.id, {
      onSuccess: () => {
        setPendingDelete(null);
        toast.success("Routine deleted");
      },
      onError: (err) =>
        toast.error(err instanceof ApiError ? err.message : "Failed to delete routine"),
    });
  }

  const list = routines.data?.routines ?? [];

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-xs text-muted-foreground">
          Routines run {botName}'s prompt on a cron schedule.
        </p>
        <Button
          size="sm"
          variant="brand"
          className="gap-1.5"
          onClick={() => {
            setEditing(null);
            setFormOpen(true);
          }}
        >
          <Plus className="size-3.5" />
          New routine
        </Button>
      </div>

      {routines.isLoading && <Skeleton className="h-44 rounded-2xl" />}

      {list.map((routine) => (
        <div
          key={routine.id}
          className="flex flex-col gap-3 rounded-2xl border border-border/60 bg-card p-4 sm:flex-row sm:items-center"
        >
          <div className="min-w-0 flex-1 space-y-1">
            <div className="flex items-center gap-2">
              <h3 className="truncate text-sm font-semibold">{routine.name}</h3>
              <Badge variant="outline" className="text-[10px] text-muted-foreground">
                {routine.timezone}
              </Badge>
            </div>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
              <code className="font-mono">{routine.schedule}</code>
              <span>
                Next run:{" "}
                {routine.nextRunAt
                  ? new Date(routine.nextRunAt).toLocaleString()
                  : "—"}
              </span>
              {routine.lastRunAt && (
                <span>Last run: {new Date(routine.lastRunAt).toLocaleString()}</span>
              )}
            </div>
          </div>

          <div className="flex items-center gap-1.5 shrink-0">
            <Switch
              checked={routine.enabled}
              onCheckedChange={(checked) => handleToggle(routine, checked)}
              disabled={updateRoutine.isPending}
              aria-label={`Enable ${routine.name}`}
            />
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() => handleRunNow(routine)}
              disabled={runNow.isPending}
              aria-label={`Run ${routine.name} now`}
              title="Run now"
            >
              {runNow.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Play className="size-4" />
              )}
            </Button>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() => {
                setEditing(routine);
                setFormOpen(true);
              }}
              aria-label={`Edit ${routine.name}`}
              title="Edit routine"
            >
              <Pencil className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-xs"
              className="text-muted-foreground hover:text-destructive"
              onClick={() => setPendingDelete(routine)}
              aria-label={`Delete ${routine.name}`}
              title="Delete routine"
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        </div>
      ))}

      {!routines.isLoading && list.length === 0 && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-2xl border border-dashed border-border/80 p-12 text-center">
          <div className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
            <CalendarClock className="size-6" />
          </div>
          <div className="space-y-1">
            <h3 className="text-sm font-semibold">No routines yet</h3>
            <p className="max-w-sm text-xs text-muted-foreground">
              Routines let {botName} work on a schedule — daily digests, weekly reports, and
              more.
            </p>
          </div>
        </div>
      )}

      <RoutineFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        botId={botId}
        routine={editing ?? undefined}
      />

      <Dialog
        open={pendingDelete !== null}
        onOpenChange={(open) => !open && setPendingDelete(null)}
      >
        <DialogContent showCloseButton={false} className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete routine?</DialogTitle>
            <DialogDescription>
              “{pendingDelete?.name}” will stop running and be permanently removed.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingDelete(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleteRoutine.isPending}>
              {deleteRoutine.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}


// WorkspacePanel shows the bot's Files and Memory: files are metadata-only
// (bots write them via write_file), memories can be deleted inline.
function WorkspacePanel({ botId }: { botId: string }) {
  const files = useBotFiles(botId);
  const memories = useBotMemories(botId);
  const deleteMemory = useDeleteBotMemory(botId);

  const [viewing, setViewing] = useState<BotFileMeta | null>(null);

  const fileList = files.data?.files ?? [];
  const memoryList = memories.data?.memories ?? [];

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Files</CardTitle>
          <CardDescription>Bots write these via the write_file tool.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {files.isLoading && <Skeleton className="h-32 rounded-2xl" />}

          {fileList.map((f) => (
            <button
              key={f.path}
              type="button"
              onClick={() => setViewing(f)}
              className="flex w-full items-center justify-between gap-3 rounded-xl border border-border/60 bg-card px-3 py-2 text-left hover:bg-muted/50"
            >
              <span className="min-w-0 truncate font-mono text-xs" title={f.path}>
                {f.path}
              </span>
              <span className="shrink-0 text-[11px] text-muted-foreground">
                {formatBytes(f.sizeBytes)} · {new Date(f.updatedAt).toLocaleDateString()}
              </span>
            </button>
          ))}

          {!files.isLoading && fileList.length === 0 && (
            <div className="flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border/80 p-8 text-center">
              <FolderOpen className="size-5 text-muted-foreground" />
              <p className="text-xs text-muted-foreground">
                No files yet — give the bot a task that saves a file.
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Memory</CardTitle>
          <CardDescription>
            Learned via the remember tool — injected into every run.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {memories.isLoading && <Skeleton className="h-32 rounded-2xl" />}

          {memoryList.map((m) => (
            <div
              key={m.id}
              className="flex items-start justify-between gap-3 rounded-xl border border-border/60 bg-card px-3 py-2"
            >
              <div className="min-w-0 space-y-0.5">
                <p className="text-xs break-words">{m.content}</p>
                <p className="text-[11px] text-muted-foreground">
                  {new Date(m.createdAt).toLocaleString()}
                </p>
              </div>
              <Button
                variant="ghost"
                size="icon-xs"
                className="text-muted-foreground hover:text-destructive shrink-0"
                onClick={() =>
                  deleteMemory.mutate(m.id, {
                    onError: (err) =>
                      toast.error(
                        err instanceof ApiError ? err.message : "Failed to delete memory"
                      ),
                  })
                }
                disabled={deleteMemory.isPending}
                aria-label="Delete memory"
                title="Delete memory"
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}

          {!memories.isLoading && memoryList.length === 0 && (
            <div className="flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border/80 p-8 text-center">
              <Brain className="size-5 text-muted-foreground" />
              <p className="text-xs text-muted-foreground">
                Nothing remembered yet — ask the bot to remember something.
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <FileViewerDialog open={viewing !== null} onOpenChange={(open) => !open && setViewing(null)} file={viewing} />
    </div>
  );
}

// BotPage is the bot detail shell (header + tabs); Task 16 fills Workspace.
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

          {activeTab === "routines" && bot && (
            <RoutinesPanel botId={bot.id} botName={bot.name} />
          )}

          {activeTab === "workspace" && bot && <WorkspacePanel botId={bot.id} />}
        </div>
      </div>

      <BotFormDialog open={editOpen} onOpenChange={setEditOpen} bot={bot} />
    </div>
  );
}
