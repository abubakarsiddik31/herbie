import { useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  FolderGit2,
  Gauge,
  LogOut,
  MessageSquare,
  PanelLeftClose,
  Pencil,
  Plus,
  Search,
  Settings,
  Share2,
  Trash2,
  Workflow as WorkflowIcon,
  Wrench,
  X,
} from "lucide-react";
import { ApiError, apiFetch } from "@/lib/api";
import { useDebouncedValue } from "@/lib/useDebouncedValue";
import { cn } from "@/lib/utils";
import type { Conversation } from "@/lib/types";
import { useAuth } from "@/stores/auth";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { BrandMark } from "@/components/BrandMark";
import { ThemeToggle } from "@/components/ThemeToggle";
import { ShareDialog } from "@/features/chat/ShareDialog";
import { useConversations, useDeleteConversation } from "@/features/chat/useConversations";
import { useTools } from "@/features/tools/useTools";
import { useProjects } from "@/features/projects/useProjects";
import { useWorkflows } from "@/features/workflows/useWorkflows";
import { useSidebar } from "./SidebarContext";

const GROUP_ORDER = ["Today", "Yesterday", "Previous 7 days", "Older"] as const;

function groupKey(updatedAt: string): string {
  const d = new Date(updatedAt);
  const now = new Date();
  const startOfDay = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const days = Math.floor((startOfDay(now) - startOfDay(d)) / 86_400_000);
  if (days <= 0) return "Today";
  if (days === 1) return "Yesterday";
  if (days < 7) return "Previous 7 days";
  return "Older";
}

export function AppSidebar() {
  const navigate = useNavigate();
  const location = useLocation();
  const queryClient = useQueryClient();
  const user = useAuth((s) => s.user);
  const { collapsed, toggleSidebar, mobileOpen, setMobileOpen, openSettings } = useSidebar();

  async function handleLogout() {
    try {
      await apiFetch("/api/auth/logout", { method: "POST" });
    } catch { /* clearing local state regardless */ }
    useAuth.getState().clear();
    navigate("/login", { replace: true });
  }

  const [filter, setFilter] = useState("");
  const debouncedFilter = useDebouncedValue(filter);

  // Dialog states
  const [renaming, setRenaming] = useState<Conversation | null>(null);
  const [renameTitle, setRenameTitle] = useState("");
  const [sharing, setSharing] = useState<Conversation | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Conversation | null>(null);

  const { data: conversations, isLoading: conversationsLoading } = useConversations(debouncedFilter.trim());
  const { data: tools } = useTools();
  const { data: projects } = useProjects();
  const { data: workflows } = useWorkflows();
  const deleteConversation = useDeleteConversation();

  const renameMutation = useMutation({
    mutationFn: ({ id, title }: { id: string; title: string }) =>
      apiFetch<Conversation>(`/api/conversations/${id}`, { method: "PATCH", json: { title } }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["conversations"] }),
    onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to rename conversation"),
  });

  // Determine active conversation id from route
  const chatMatch = location.pathname.match(/^\/chat\/([^/]+)$/);
  const selectedId = chatMatch ? chatMatch[1] : null;

  function startNewChat() {
    setMobileOpen(false);
    navigate("/chat");
  }

  function selectConversation(id: string) {
    setMobileOpen(false);
    navigate(`/chat/${id}`);
  }

  function openRename(c: Conversation) {
    setRenaming(c);
    setRenameTitle(c.title || "");
  }

  function confirmRename() {
    if (!renaming || !renameTitle.trim()) return;
    renameMutation.mutate(
      { id: renaming.id, title: renameTitle.trim() },
      { onSuccess: () => setRenaming(null) }
    );
  }

  function confirmDelete() {
    if (!pendingDelete) return;
    deleteConversation.mutate(pendingDelete.id, {
      onSuccess: () => {
        if (selectedId === pendingDelete.id) {
          navigate("/chat", { replace: true });
        }
        setPendingDelete(null);
      },
      onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to delete conversation"),
    });
  }

  // Group conversations
  const groups: Array<{ label: string; items: Conversation[] }> = [];
  if (conversations) {
    const buckets: Record<string, Conversation[]> = {};
    for (const c of conversations) {
      const key = groupKey(c.updatedAt);
      (buckets[key] ??= []).push(c);
    }
    for (const label of GROUP_ORDER) {
      const items = buckets[label];
      if (items && items.length > 0) groups.push({ label, items });
    }
  }

  const nothingMatches =
    !conversationsLoading &&
    debouncedFilter.trim() !== "" &&
    groups.every((g) => g.items.length === 0);

  const isNavActive = (path: string) => {
    if (path === "/chat") {
      return location.pathname.startsWith("/chat");
    }
    return location.pathname.startsWith(path);
  };

  return (
    <>
      {/* Mobile backdrop */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 backdrop-blur-xs md:hidden"
          onClick={() => setMobileOpen(false)}
        />
      )}

      <aside
        className={cn(
          "flex h-dvh flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground transition-all duration-200 z-50",
          // Desktop collapse
          collapsed ? "w-0 -translate-x-full border-r-0 md:w-0 overflow-hidden" : "w-64 translate-x-0",
          // Mobile drawer
          "max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:shadow-2xl",
          !mobileOpen && "max-md:-translate-x-full"
        )}
      >
        {/* Brand & collapse button */}
        <div className="flex items-center justify-between p-3 border-b border-sidebar-border/60">
          <div className="flex items-center gap-2.5">
            <BrandMark className="size-6" />
            <span className="font-semibold text-sm tracking-tight">Herbie</span>
          </div>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon-xs"
              className="hidden md:flex text-muted-foreground hover:text-foreground"
              aria-label="Collapse sidebar (⌘B)"
              title="Collapse sidebar (⌘B)"
              onClick={toggleSidebar}
            >
              <PanelLeftClose className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-xs"
              className="md:hidden text-muted-foreground hover:text-foreground"
              aria-label="Close sidebar"
              onClick={() => setMobileOpen(false)}
            >
              <X className="size-4" />
            </Button>
          </div>
        </div>

        {/* New chat action */}
        <div className="p-3 pb-2">
          <Button
            size="sm"
            variant="brand"
            onClick={startNewChat}
            className="w-full justify-between shadow-xs hover:shadow-sm"
          >
            <span className="flex items-center gap-2">
              <Plus className="size-4" />
              <span>New chat</span>
            </span>
            <kbd className="font-mono text-[10px] bg-brand-foreground/20 px-1 py-0.5 rounded text-brand-foreground">
              ⌘O
            </kbd>
          </Button>
        </div>

        {/* Quick Nav Links */}
        <div className="px-3 pb-2 space-y-0.5 border-b border-sidebar-border/40">
          <Link
            to="/chat"
            onClick={() => setMobileOpen(false)}
            className={cn(
              "flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors",
              isNavActive("/chat") && !selectedId
                ? "bg-sidebar-accent text-sidebar-accent-foreground font-semibold"
                : "text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
            )}
          >
            <MessageSquare className="size-3.5 shrink-0" />
            <span className="flex-1">Chat</span>
          </Link>

          <Link
            to="/projects"
            onClick={() => setMobileOpen(false)}
            className={cn(
              "flex items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors",
              isNavActive("/projects")
                ? "bg-sidebar-accent text-sidebar-accent-foreground font-semibold"
                : "text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
            )}
          >
            <span className="flex items-center gap-2.5">
              <FolderGit2 className="size-3.5 shrink-0" />
              <span>Projects</span>
            </span>
            {projects && projects.length > 0 && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono h-4">
                {projects.length}
              </Badge>
            )}
          </Link>

          <Link
            to="/workflows"
            onClick={() => setMobileOpen(false)}
            className={cn(
              "flex items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors",
              isNavActive("/workflows")
                ? "bg-sidebar-accent text-sidebar-accent-foreground font-semibold"
                : "text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
            )}
          >
            <span className="flex items-center gap-2.5">
              <WorkflowIcon className="size-3.5 shrink-0" />
              <span>Workflows</span>
            </span>
            {workflows && workflows.length > 0 && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono h-4">
                {workflows.length}
              </Badge>
            )}
          </Link>

          <Link
            to="/tools"
            onClick={() => setMobileOpen(false)}
            className={cn(
              "flex items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors",
              isNavActive("/tools")
                ? "bg-sidebar-accent text-sidebar-accent-foreground font-semibold"
                : "text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
            )}
          >
            <span className="flex items-center gap-2.5">
              <Wrench className="size-3.5 shrink-0" />
              <span>Tools</span>
            </span>
            {tools && tools.length > 0 && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono h-4">
                {tools.length}
              </Badge>
            )}
          </Link>

          <Link
            to="/usage"
            onClick={() => setMobileOpen(false)}
            className={cn(
              "flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors",
              isNavActive("/usage")
                ? "bg-sidebar-accent text-sidebar-accent-foreground font-semibold"
                : "text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
            )}
          >
            <Gauge className="size-3.5 shrink-0" />
            <span className="flex-1">Usage</span>
          </Link>
        </div>

        {/* Search bar */}
        <div className="p-3 pb-2">
          <div className="relative">
            <Search className="absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground/70" />
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Search chats…"
              aria-label="Search conversations"
              className="h-8 w-full rounded-lg border border-sidebar-border bg-background/50 pr-7 pl-8 text-xs outline-none placeholder:text-muted-foreground/60 focus-visible:border-sidebar-ring focus-visible:ring-[2px] focus-visible:ring-sidebar-ring/40"
            />
            {filter && (
              <button
                type="button"
                onClick={() => setFilter("")}
                className="absolute top-1/2 right-2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              >
                <X className="size-3" />
              </button>
            )}
          </div>
        </div>

        {/* Conversation history */}
        <ScrollArea className="min-h-0 flex-1 px-2">
          <nav className="space-y-0.5 pb-3">
            {conversationsLoading && (
              <div className="space-y-2 px-1 pt-1">
                {[0, 1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-8 w-full rounded-md" />
                ))}
              </div>
            )}
            {!conversationsLoading && (conversations?.length ?? 0) === 0 && debouncedFilter.trim() === "" && (
              <p className="px-3 py-6 text-center text-muted-foreground text-xs">
                No conversations yet. Start a new chat above.
              </p>
            )}
            {nothingMatches && (
              <p className="px-3 py-6 text-center text-muted-foreground text-xs">No conversations match.</p>
            )}
            {groups.map((group) => (
              <div key={group.label} className="pt-2">
                <p className="px-2 pb-1 text-[10px] font-semibold tracking-wider text-muted-foreground/60 uppercase">
                  {group.label}
                </p>
                {group.items.map((c) => {
                  const isSelected = c.id === selectedId;
                  return (
                    <div
                      key={c.id}
                      className={cn(
                        "group relative flex items-center rounded-lg px-2 py-1 text-xs transition-colors",
                        isSelected
                          ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium border-l-2 border-brand pl-1.5"
                          : "text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
                      )}
                    >
                      <button
                        type="button"
                        onClick={() => selectConversation(c.id)}
                        title={c.title || "Untitled"}
                        className="min-w-0 flex-1 truncate text-left outline-none py-1"
                      >
                        {c.title || "Untitled"}
                      </button>
                      <div className="hidden shrink-0 items-center gap-0.5 group-hover:flex group-focus-within:flex pl-1">
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          aria-label={`Rename ${c.title || "conversation"}`}
                          onClick={(e) => {
                            e.stopPropagation();
                            openRename(c);
                          }}
                        >
                          <Pencil className="size-3" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          aria-label={`Share ${c.title || "conversation"}`}
                          onClick={(e) => {
                            e.stopPropagation();
                            setSharing(c);
                          }}
                        >
                          <Share2 className="size-3" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          aria-label={`Delete ${c.title || "conversation"}`}
                          className="text-destructive hover:text-destructive"
                          onClick={(e) => {
                            e.stopPropagation();
                            setPendingDelete(c);
                          }}
                        >
                          <Trash2 className="size-3" />
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            ))}
          </nav>
        </ScrollArea>

        {/* User profile & settings footer */}
        <div className="border-t border-sidebar-border/60 p-2.5">
          <div className="flex items-center gap-2">
            <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold uppercase text-primary">
              {(user?.email ?? "?").slice(0, 1)}
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-xs font-medium text-sidebar-foreground" title={user?.email}>
                {user?.email ?? "Signed in"}
              </p>
            </div>
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label="Settings"
              title="Settings"
              onClick={() => openSettings("general")}
              className="text-muted-foreground hover:text-foreground"
            >
              <Settings className="size-3.5" />
            </Button>
            <ThemeToggle />
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label="Log out"
              title="Log out"
              onClick={() => void handleLogout()}
              className="text-muted-foreground hover:text-destructive"
            >
              <LogOut className="size-3.5" />
            </Button>
          </div>
        </div>
      </aside>

      {/* Rename Dialog */}
      <Dialog open={renaming !== null} onOpenChange={(open) => { if (!open) setRenaming(null); }}>
        <DialogContent showCloseButton={false} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Rename conversation</DialogTitle>
            <DialogDescription>Give this conversation a title you'll recognize.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="sidebar-rename-title">Title</Label>
            <Input
              id="sidebar-rename-title"
              value={renameTitle}
              onChange={(e) => setRenameTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") confirmRename();
              }}
              autoFocus
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenaming(null)}>Cancel</Button>
            <Button onClick={confirmRename} disabled={renameMutation.isPending || !renameTitle.trim()}>
              {renameMutation.isPending ? "Saving…" : "Save"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Share Dialog */}
      <ShareDialog conversation={sharing} onOpenChange={(open) => { if (!open) setSharing(null); }} />

      {/* Delete Confirmation Dialog */}
      <Dialog open={pendingDelete !== null} onOpenChange={(open) => { if (!open) setPendingDelete(null); }}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Delete conversation?</DialogTitle>
            <DialogDescription>
              “{pendingDelete?.title || "Untitled"}” and all of its messages will be permanently removed.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingDelete(null)}>Cancel</Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={deleteConversation.isPending}>
              {deleteConversation.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
