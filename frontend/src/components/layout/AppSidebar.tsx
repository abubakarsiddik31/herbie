import { useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  Calendar,
  ChevronDown,
  FileText,
  FolderGit2,
  Gauge,
  GitBranch,
  Globe,
  LogOut,
  MessageSquare,
  MoreHorizontal,
  PanelLeftClose,
  Pencil,
  Plug,
  Plus,
  Search,
  Settings,
  Share2,
  Terminal,
  Trash2,
  Workflow as WorkflowIcon,
  Wrench,
  X,
} from "lucide-react";
import { ApiError, apiFetch } from "@/lib/api";
import { useDebouncedValue } from "@/lib/useDebouncedValue";
import { cn, cleanConversationTitle, isFileTitle } from "@/lib/utils";
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { BrandMark } from "@/components/BrandMark";
import { ThemeToggle } from "@/components/ThemeToggle";
import { ShareDialog } from "@/features/chat/ShareDialog";
import { useConversations, useDeleteConversation } from "@/features/chat/useConversations";
import { useTools } from "@/features/tools/useTools";
import { useProjects, useProject, type Project } from "@/features/projects/useProjects";
import { useToolOAuthProviders, useWorkflows } from "@/features/workflows/useWorkflows";
import { useMCPServers } from "@/features/mcp/useMCPServers";
import { MCPServersDialog } from "@/features/mcp/MCPServersDialog";
import { useSidebar } from "./SidebarContext";

const GROUP_ORDER = ["Today", "Yesterday", "Previous 7 days", "Older"] as const;

/** One expandable project row in the sidebar, ChatGPT-style: chats
 *  nest under the project, fetched lazily when it expands. */
function SidebarProject({ project }: { project: Project }) {
  const navigate = useNavigate();
  const location = useLocation();
  const { setMobileOpen } = useSidebar();
  const [expanded, setExpanded] = useState(false);
  // Details (chats) load only while the project is expanded.
  const { data: detail, isLoading } = useProject(expanded ? project.id : null);

  const workspacePath = `/projects/${project.id}`;
  const inWorkspace = location.pathname === workspacePath;
  const activeConv = inWorkspace ? new URLSearchParams(location.search).get("c") : null;

  function open(target: { conv?: string } = {}) {
    setMobileOpen(false);
    navigate(target.conv ? `${workspacePath}?c=${target.conv}` : workspacePath);
  }

  return (
    <div>
      <div
        className={cn(
          "group flex items-center rounded-lg text-xs transition-colors",
          inWorkspace
            ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
            : "text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
        )}
      >
        <button
          type="button"
          onClick={() => setExpanded((e) => !e)}
          aria-expanded={expanded}
          aria-label={`${expanded ? "Collapse" : "Expand"} ${project.name}`}
          className="flex shrink-0 items-center p-1.5 pl-2 outline-none"
        >
          <ChevronDown
            className={cn("size-3 text-muted-foreground/70 transition-transform", !expanded && "-rotate-90")}
          />
        </button>
        <button
          type="button"
          onClick={() => open()}
          title={project.name}
          className="min-w-0 flex-1 truncate py-1.5 pr-2 text-left outline-none"
        >
          {project.name}
        </button>
      </div>

      {expanded && (
        <div className="ml-4 pl-2 border-l border-sidebar-border/60 py-1 space-y-0.5 animate-in fade-in duration-150">
          {isLoading && (
            <div className="space-y-1.5 px-1 py-1">
              <Skeleton className="h-5 w-4/5 rounded-md" />
              <Skeleton className="h-5 w-3/5 rounded-md" />
            </div>
          )}
          {detail && (
            <>
              {detail.conversations.length === 0 ? (
                <p className="px-2 py-0.5 text-[11px] text-muted-foreground/60">No chats yet</p>
              ) : (
                detail.conversations.map((c) => {
                  const title = cleanConversationTitle(c.title);
                  const isFile = isFileTitle(c.title);
                  const isActive = activeConv === c.id;
                  return (
                    <button
                      key={c.id}
                      type="button"
                      onClick={() => open({ conv: c.id })}
                      className={cn(
                        "flex w-full items-center gap-2 rounded-md px-2 py-1 text-[11px] transition-colors",
                        isActive
                          ? "bg-sidebar-accent/60 text-sidebar-accent-foreground font-medium"
                          : "text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
                      )}
                    >
                      {isFile ? (
                        <FileText
                          className={cn(
                            "size-3 shrink-0 transition-colors",
                            isActive ? "text-primary" : "text-primary/70"
                          )}
                        />
                      ) : (
                        <MessageSquare
                          className={cn(
                            "size-3 shrink-0 transition-opacity",
                            isActive ? "opacity-100 text-sidebar-accent-foreground" : "opacity-70"
                          )}
                        />
                      )}
                      <span className="min-w-0 flex-1 truncate text-left">{title}</span>
                    </button>
                  );
                })
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}

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
  const [openMenuId, setOpenMenuId] = useState<string | null>(null);

  const { data: conversations, isLoading: conversationsLoading } = useConversations(debouncedFilter.trim());
  const { data: tools } = useTools();
  const { data: projects } = useProjects();
  const { data: workflows } = useWorkflows();
  const { data: oauthProviders } = useToolOAuthProviders();
  const { data: mcpServers } = useMCPServers();
  const [mcpDialogOpen, setMcpDialogOpen] = useState(false);
  const [appsExpanded, setAppsExpanded] = useState(true);
  const deleteConversation = useDeleteConversation();

  const gcalProvider = oauthProviders?.find((p) => p.id === "google_calendar");
  const ghProvider = oauthProviders?.find((p) => p.id === "github");
  const slackProvider = oauthProviders?.find((p) => p.id === "slack");

  const gcalConnected = gcalProvider?.connected;
  const ghConnected = ghProvider?.connected;
  const slackConnected = slackProvider?.connected;
  const webFetchConnected = mcpServers?.some((s) => s.appId === "web_fetch" && s.enabled);
  const codeRunnerConnected = mcpServers?.some((s) => s.appId === "code_runner" && s.enabled);

  function handleTriggerApp(mention: string) {
    setMobileOpen(false);
    navigate(`/chat?mention=${mention}`);
  }

  const renameMutation = useMutation({
    mutationFn: ({ id, title }: { id: string; title: string }) =>
      apiFetch<Conversation>(`/api/conversations/${id}`, { method: "PATCH", json: { title } }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["conversations"] }),
    onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to rename conversation"),
  });

  // Determine active conversation id from route
  const chatMatch = location.pathname.match(/^\/chat\/([^/]+)$/);
  const selectedId = chatMatch ? chatMatch[1] : null;

  const projectMatch = location.pathname.match(/^\/projects\/([^/]+)$/);
  const activeProjectId = projectMatch ? projectMatch[1] : null;

  function startNewChat() {
    setMobileOpen(false);
    if (activeProjectId) {
      navigate(`/projects/${activeProjectId}?c=new`);
    } else {
      navigate("/chat");
    }
  }

  function selectConversation(id: string) {
    setMobileOpen(false);
    navigate(`/chat/${id}`);
  }

  function openRename(c: Conversation) {
    setRenaming(c);
    setRenameTitle(cleanConversationTitle(c.title));
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
              <span>{activeProjectId ? "New project chat" : "New chat"}</span>
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

          {/* Apps & MCP in the same area as Projects and Workflows */}
          <div className="space-y-0.5">
            <button
              type="button"
              onClick={() => setAppsExpanded(!appsExpanded)}
              className={cn(
                "flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors",
                appsExpanded
                  ? "bg-sidebar-accent/60 text-sidebar-accent-foreground font-semibold"
                  : "text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
              )}
            >
              <span className="flex items-center gap-2.5">
                <Plug className="size-3.5 shrink-0" />
                <span>Apps & MCP</span>
                <ChevronDown
                  className={cn(
                    "size-3 text-muted-foreground/70 transition-transform duration-200",
                    appsExpanded && "rotate-180"
                  )}
                />
              </span>
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono h-4 shrink-0">
                {(gcalConnected ? 1 : 0) +
                  (ghConnected ? 1 : 0) +
                  (slackConnected ? 1 : 0) +
                  1 +
                  (webFetchConnected ? 1 : 0) +
                  (codeRunnerConnected ? 1 : 0) +
                  (mcpServers?.filter((s) => s.enabled && !s.appId).length ?? 0)}
              </Badge>
            </button>

            {appsExpanded && (
              <div className="ml-3 pl-2.5 border-l border-sidebar-border/60 py-1 space-y-0.5 animate-in fade-in duration-150">
                {/* Google Calendar (only if connected) */}
                {gcalConnected && (
                  <button
                    type="button"
                    onClick={() => handleTriggerApp("calendar")}
                    className="flex w-full items-center justify-between rounded-md px-2 py-1 text-[11px] text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground group"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <Calendar className="size-3 shrink-0 text-sky-500" />
                      <span className="truncate">Google Calendar</span>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      <span className="text-[10px] font-mono text-muted-foreground opacity-0 group-hover:opacity-100">@calendar</span>
                      <Badge variant="outline" className="text-[9px] font-mono px-1 py-0 h-3.5 border-emerald-500/30 text-emerald-600 dark:text-emerald-400">
                        {gcalProvider?.connectedVia === "mcp" ? "MCP" : "OAuth"}
                      </Badge>
                      <span className="size-1.5 rounded-full bg-emerald-500 shadow-[0_0_4px_rgba(16,185,129,0.5)]" title="Connected" />
                    </div>
                  </button>
                )}

                {/* GitHub (only if connected) */}
                {ghConnected && (
                  <button
                    type="button"
                    onClick={() => handleTriggerApp("github")}
                    className="flex w-full items-center justify-between rounded-md px-2 py-1 text-[11px] text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground group"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <GitBranch className="size-3 shrink-0 text-neutral-700 dark:text-neutral-300" />
                      <span className="truncate">GitHub</span>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      <span className="text-[10px] font-mono text-muted-foreground opacity-0 group-hover:opacity-100">@github</span>
                      <Badge variant="outline" className="text-[9px] font-mono px-1 py-0 h-3.5 border-emerald-500/30 text-emerald-600 dark:text-emerald-400">
                        {ghProvider?.connectedVia === "mcp" ? "MCP" : "OAuth"}
                      </Badge>
                      <span className="size-1.5 rounded-full bg-emerald-500 shadow-[0_0_4px_rgba(16,185,129,0.5)]" title="Connected" />
                    </div>
                  </button>
                )}

                {/* Slack (only if connected) */}
                {slackConnected && (
                  <button
                    type="button"
                    onClick={() => handleTriggerApp("slack")}
                    className="flex w-full items-center justify-between rounded-md px-2 py-1 text-[11px] text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground group"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <MessageSquare className="size-3 shrink-0 text-emerald-500" />
                      <span className="truncate">Slack</span>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      <span className="text-[10px] font-mono text-muted-foreground opacity-0 group-hover:opacity-100">@slack</span>
                      <Badge variant="outline" className="text-[9px] font-mono px-1 py-0 h-3.5 border-emerald-500/30 text-emerald-600 dark:text-emerald-400">
                        {slackProvider?.connectedVia === "mcp" ? "MCP" : "OAuth"}
                      </Badge>
                      <span className="size-1.5 rounded-full bg-emerald-500 shadow-[0_0_4px_rgba(16,185,129,0.5)]" title="Connected" />
                    </div>
                  </button>
                )}

                {/* Web Search (Built-in) */}
                <button
                  type="button"
                  onClick={() => handleTriggerApp("web")}
                  className="flex w-full items-center justify-between rounded-md px-2 py-1 text-[11px] text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground group"
                >
                  <div className="flex items-center gap-2 min-w-0">
                    <Globe className="size-3 shrink-0 text-cyan-500" />
                    <span className="truncate">Web Search</span>
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    <span className="text-[10px] font-mono text-muted-foreground opacity-0 group-hover:opacity-100">@web</span>
                    <span className="size-1.5 rounded-full bg-cyan-500" title="Built-in" />
                  </div>
                </button>

                {/* Web Reader & Fetch (only if connected) */}
                {webFetchConnected && (
                  <button
                    type="button"
                    onClick={() => handleTriggerApp("web_fetch")}
                    className="flex w-full items-center justify-between rounded-md px-2 py-1 text-[11px] text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground group"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <Globe className="size-3 shrink-0 text-blue-500" />
                      <span className="truncate">Web Reader</span>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      <span className="text-[10px] font-mono text-muted-foreground opacity-0 group-hover:opacity-100">@web_fetch</span>
                      <Badge variant="outline" className="text-[9px] font-mono px-1 py-0 h-3.5 border-emerald-500/30 text-emerald-600 dark:text-emerald-400">
                        MCP
                      </Badge>
                      <span className="size-1.5 rounded-full bg-emerald-500" title="Connected" />
                    </div>
                  </button>
                )}

                {/* Code Sandbox (only if connected) */}
                {codeRunnerConnected && (
                  <button
                    type="button"
                    onClick={() => handleTriggerApp("code_runner")}
                    className="flex w-full items-center justify-between rounded-md px-2 py-1 text-[11px] text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground group"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <Terminal className="size-3 shrink-0 text-amber-500" />
                      <span className="truncate">Code Sandbox</span>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      <span className="text-[10px] font-mono text-muted-foreground opacity-0 group-hover:opacity-100">@code_runner</span>
                      <Badge variant="outline" className="text-[9px] font-mono px-1 py-0 h-3.5 border-emerald-500/30 text-emerald-600 dark:text-emerald-400">
                        MCP
                      </Badge>
                      <span className="size-1.5 rounded-full bg-emerald-500" title="Connected" />
                    </div>
                  </button>
                )}

                {/* Standalone MCP Servers (only enabled ones) */}
                {mcpServers?.filter((s) => s.enabled && !s.appId).map((srv) => (
                  <button
                    key={srv.id}
                    type="button"
                    onClick={() => handleTriggerApp(srv.name)}
                    className="flex w-full items-center justify-between rounded-md px-2 py-1 text-[11px] text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground group"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <Plug className="size-3 shrink-0 text-muted-foreground" />
                      <span className="truncate">{srv.name}</span>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      <Badge variant="outline" className="text-[9px] font-mono px-1 py-0 h-3.5 border-emerald-500/30 text-emerald-600 dark:text-emerald-400">
                        MCP
                      </Badge>
                      <span className="size-1.5 rounded-full bg-emerald-500" title="Connected" />
                    </div>
                  </button>
                ))}

                {/* Button that opens Settings modal Apps/Tools tab */}
                <button
                  type="button"
                  onClick={() => {
                    setMobileOpen(false);
                    openSettings("tools");
                  }}
                  className="flex w-full items-center justify-between rounded-md px-2 py-1.5 text-[11px] text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground group pt-1.5 border-t border-sidebar-border/40 mt-1"
                >
                  <div className="flex items-center gap-2 min-w-0">
                    <Plus className="size-3 shrink-0 text-muted-foreground" />
                    <span className="truncate font-medium text-foreground">Add & Manage Apps</span>
                  </div>
                  <Badge variant="outline" className="px-1.5 py-0 text-[9px] font-mono h-4 text-muted-foreground">
                    Manage
                  </Badge>
                </button>
              </div>
            )}
          </div>

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
            {projects && projects.length > 0 && (
              <div className="pt-2">
                <p className="px-2 pb-1 text-[10px] font-semibold tracking-wider text-muted-foreground/60 uppercase">
                  Projects
                </p>
                <div className="space-y-0.5">
                  {projects.map((p) => (
                    <SidebarProject key={p.id} project={p} />
                  ))}
                </div>
              </div>
            )}
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
                  const displayTitle = cleanConversationTitle(c.title);
                  const isFile = isFileTitle(c.title);
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
                        className="flex items-center gap-2 min-w-0 flex-1 truncate text-left outline-none py-1 pr-7"
                      >
                        {isFile ? (
                          <FileText
                            className={cn(
                              "size-3.5 shrink-0 transition-colors",
                              isSelected
                                ? "text-primary"
                                : "text-primary/70 group-hover:text-primary"
                            )}
                          />
                        ) : (
                          <MessageSquare
                            className={cn(
                              "size-3.5 shrink-0 transition-opacity",
                              isSelected
                                ? "opacity-100 text-sidebar-accent-foreground"
                                : "opacity-60 group-hover:opacity-100"
                            )}
                          />
                        )}
                        <span className="truncate">{displayTitle}</span>
                      </button>
                      <div
                        className={cn(
                          "absolute right-1 top-1/2 -translate-y-1/2 flex items-center z-10 transition-opacity",
                          openMenuId === c.id
                            ? "opacity-100"
                            : "opacity-0 group-hover:opacity-100 focus-within:opacity-100 max-md:opacity-75"
                        )}
                      >
                        <DropdownMenu
                          open={openMenuId === c.id}
                          onOpenChange={(open) => setOpenMenuId(open ? c.id : null)}
                        >
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              aria-label={`Options for ${displayTitle}`}
                              className={cn(
                                "size-6 rounded-md hover:bg-background/80 hover:text-foreground text-muted-foreground transition-all",
                                isSelected ? "text-sidebar-accent-foreground" : ""
                              )}
                              onClick={(e) => e.stopPropagation()}
                            >
                              <MoreHorizontal className="size-3.5" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-44">
                            <DropdownMenuItem
                              aria-label={`Rename ${displayTitle}`}
                              onClick={(e) => {
                                e.stopPropagation();
                                openRename(c);
                              }}
                            >
                              <Pencil className="size-3.5 text-muted-foreground" />
                              <span>Rename</span>
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              aria-label={`Share ${displayTitle}`}
                              onClick={(e) => {
                                e.stopPropagation();
                                setSharing(c);
                              }}
                            >
                              <Share2 className="size-3.5 text-muted-foreground" />
                              <span>Share link</span>
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              variant="destructive"
                              aria-label={`Delete ${displayTitle}`}
                              onClick={(e) => {
                                e.stopPropagation();
                                setPendingDelete(c);
                              }}
                            >
                              <Trash2 className="size-3.5 text-destructive" />
                              <span>Delete chat</span>
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
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

      {/* MCP Servers Dialog */}
      <MCPServersDialog open={mcpDialogOpen} onOpenChange={setMcpDialogOpen} />

      {/* Share Dialog */}
      <ShareDialog conversation={sharing} onOpenChange={(open) => { if (!open) setSharing(null); }} />

      {/* Delete Confirmation Dialog */}
      <Dialog open={pendingDelete !== null} onOpenChange={(open) => { if (!open) setPendingDelete(null); }}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Delete conversation?</DialogTitle>
            <DialogDescription>
              “{cleanConversationTitle(pendingDelete?.title)}” and all of its messages will be permanently removed.
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
