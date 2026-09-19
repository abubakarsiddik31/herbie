import React, { useState, useEffect } from "react";
import { Link } from "react-router";
import { useTheme } from "next-themes";
import { toast } from "sonner";
import {
  Brain,
  Calendar,
  CheckCircle2,
  FileText,
  Gauge,
  GitBranch,
  Globe,
  Laptop,
  MessageSquare,
  Moon,
  Plug,
  Plus,
  Power,
  Settings,
  ShieldCheck,
  Sparkles,
  Sun,
  Terminal,
  Trash2,
  User,
  Wrench,
  X,
} from "lucide-react";
import { ApiError, apiFetch } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useAuth } from "@/stores/auth";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { useProfile, useUpdateProfile } from "@/features/profile/useProfile";
import {
  useMemories,
  useCreateMemory,
  useDeleteMemory,
  useClearMemories,
} from "@/features/memories/useMemories";
import { useTools, useUpdateTool } from "@/features/tools/useTools";
import { useDocuments } from "@/features/documents/useDocuments";
import { useToolOAuthProviders, useWorkflows } from "@/features/workflows/useWorkflows";
import { useMCPServers } from "@/features/mcp/useMCPServers";
import { LinkAppDialog } from "@/features/mcp/LinkAppDialog";
import { useLinkCatalogApp, useUnlinkCatalogApp } from "@/features/mcp/useMCPCatalog";

export const MAX_INSTRUCTIONS_CHARS = 4000;
export const MAX_MEMORY_CHARS = 1000;

export type SettingsTab = "general" | "personalization" | "tools" | "documents" | "account";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultTab?: SettingsTab;
}

export function SettingsDialog({ open, onOpenChange, defaultTab = "general" }: Props) {
  const [activeTab, setActiveTab] = useState<SettingsTab>(defaultTab);
  const { theme, setTheme } = useTheme();
  const user = useAuth((s) => s.user);

  async function handleLogout() {
    try {
      await apiFetch("/api/auth/logout", { method: "POST" });
    } catch { /* clearing local state regardless */ }
    useAuth.getState().clear();
    window.location.href = "/login";
  }

  // Profile / Custom instructions
  const { data: profileData, isLoading: profileLoading } = useProfile();
  const updateProfile = useUpdateProfile();
  const [instructionsDraft, setInstructionsDraft] = useState<string | null>(null);

  // Memories
  const { data: memories, isLoading: memoriesLoading } = useMemories();
  const createMemory = useCreateMemory();
  const deleteMemory = useDeleteMemory();
  const clearMemories = useClearMemories();
  const [newMemory, setNewMemory] = useState("");

  // Tools, Apps, Workflows & MCP
  const { data: tools } = useTools();
  const updateTool = useUpdateTool();
  const { data: oauthProviders } = useToolOAuthProviders();
  const { data: workflows } = useWorkflows();
  const { data: mcpServers } = useMCPServers();
  const { data: docs } = useDocuments();
  const linkCatalogApp = useLinkCatalogApp();
  const unlinkCatalogApp = useUnlinkCatalogApp();
  const [linkAppDialogOpen, setLinkAppDialogOpen] = useState(false);
  const [linkAppInitialId, setLinkAppInitialId] = useState<string>("github");

  function openLinkApp(id: string) {
    setLinkAppInitialId(id);
    setLinkAppDialogOpen(true);
  }

  useEffect(() => {
    if (open) {
      setActiveTab(defaultTab);
      setInstructionsDraft(null);
    }
  }, [open, defaultTab]);

  const currentInstructions =
    instructionsDraft !== null ? instructionsDraft : (profileData?.defaultInstructions ?? "");
  const dirtyInstructions =
    instructionsDraft !== null &&
    profileData !== undefined &&
    instructionsDraft !== profileData.defaultInstructions;
  const instructionsTooLong = currentInstructions.length > MAX_INSTRUCTIONS_CHARS;

  function saveInstructions() {
    if (instructionsDraft === null) return;
    updateProfile.mutate(instructionsDraft, {
      onSuccess: () => toast.success("Custom instructions saved"),
      onError: (err) =>
        toast.error(err instanceof ApiError ? err.message : "Failed to save preferences"),
    });
  }

  function handleAddMemory(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = newMemory.trim();
    if (!trimmed) return;
    if (trimmed.length > MAX_MEMORY_CHARS) {
      toast.error(`Memory cannot exceed ${MAX_MEMORY_CHARS} characters`);
      return;
    }
    createMemory.mutate(trimmed, {
      onSuccess: () => {
        setNewMemory("");
        toast.success("Memory saved");
      },
      onError: (err) =>
        toast.error(err instanceof ApiError ? err.message : "Failed to save memory"),
    });
  }

  const tabs: Array<{ id: SettingsTab; label: string; icon: React.ComponentType<{ className?: string }> }> = [
    { id: "general", label: "General", icon: Settings },
    { id: "personalization", label: "Personalization", icon: Brain },
    { id: "tools", label: "Tools", icon: Wrench },
    { id: "documents", label: "Documents", icon: FileText },
    { id: "account", label: "Account", icon: User },
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="sm:max-w-3xl p-0 overflow-hidden rounded-2xl gap-0">
        <div className="flex items-center justify-between border-b px-5 py-3.5">
          <DialogTitle className="text-base font-semibold tracking-tight">Settings</DialogTitle>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => onOpenChange(false)}
            className="rounded-full text-muted-foreground hover:text-foreground"
          >
            <X className="size-4" />
          </Button>
        </div>

        <div className="flex flex-col md:flex-row min-h-[460px] max-h-[85vh]">
          {/* Tabs Sidebar */}
          <nav className="flex md:flex-col border-b md:border-b-0 md:border-r border-border p-2 gap-1 md:w-48 shrink-0 overflow-x-auto bg-muted/20">
            {tabs.map((tab) => {
              const Icon = tab.icon;
              const isActive = activeTab === tab.id;
              return (
                <button
                  key={tab.id}
                  type="button"
                  onClick={() => setActiveTab(tab.id)}
                  className={cn(
                    "flex items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors text-left shrink-0",
                    isActive
                      ? "bg-accent text-accent-foreground shadow-xs font-semibold"
                      : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                  )}
                >
                  <Icon className="size-4 shrink-0" />
                  <span>{tab.label}</span>
                </button>
              );
            })}
          </nav>

          {/* Tab Content Area */}
          <div className="flex-1 min-w-0 overflow-y-auto p-5">
            {activeTab === "general" && (
              <div className="space-y-6">
                <div>
                  <h3 className="text-sm font-semibold text-foreground">Theme</h3>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    Customize the interface appearance across devices.
                  </p>
                  <div className="mt-3 grid grid-cols-3 gap-2">
                    {[
                      { id: "light", label: "Light", icon: Sun },
                      { id: "dark", label: "Dark", icon: Moon },
                      { id: "system", label: "System", icon: Laptop },
                    ].map((mode) => {
                      const Icon = mode.icon;
                      const isSelected = theme === mode.id;
                      return (
                        <button
                          key={mode.id}
                          type="button"
                          onClick={() => setTheme(mode.id)}
                          className={cn(
                            "flex flex-col items-center gap-2 rounded-xl border p-3 text-xs font-medium transition-all",
                            isSelected
                              ? "border-primary bg-primary/5 text-primary shadow-xs ring-1 ring-primary"
                              : "border-border/60 hover:bg-accent/50 text-muted-foreground"
                          )}
                        >
                          <Icon className="size-4" />
                          <span>{mode.label}</span>
                        </button>
                      );
                    })}
                  </div>
                </div>

                <div className="border-t pt-4">
                  <h3 className="text-sm font-semibold text-foreground">Keyboard Shortcuts</h3>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    Press <kbd className="font-mono bg-muted px-1.5 py-0.5 rounded text-[11px]">?</kbd> anywhere to view full cheat sheet.
                  </p>
                  <div className="mt-3 space-y-1.5 text-xs">
                    <div className="flex justify-between py-1 border-b border-border/40">
                      <span className="text-muted-foreground">Open Command Palette</span>
                      <kbd className="font-mono bg-muted px-1.5 py-0.5 rounded text-[11px]">⌘K / Ctrl+K</kbd>
                    </div>
                    <div className="flex justify-between py-1 border-b border-border/40">
                      <span className="text-muted-foreground">New Chat</span>
                      <kbd className="font-mono bg-muted px-1.5 py-0.5 rounded text-[11px]">⌘Shift+O</kbd>
                    </div>
                    <div className="flex justify-between py-1">
                      <span className="text-muted-foreground">Toggle Sidebar</span>
                      <kbd className="font-mono bg-muted px-1.5 py-0.5 rounded text-[11px]">⌘B / Ctrl+B</kbd>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {activeTab === "personalization" && (
              <div className="space-y-6">
                {/* Custom Instructions */}
                <div>
                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-sm font-semibold text-foreground">Custom instructions</h3>
                      <p className="text-xs text-muted-foreground mt-0.5">
                        What would you like Herbie to know about you to provide better responses?
                      </p>
                    </div>
                  </div>

                  {profileLoading ? (
                    <Skeleton className="mt-3 h-28 w-full rounded-xl" />
                  ) : (
                    <div className="mt-3 space-y-2">
                      <textarea
                        aria-label="Custom instructions"
                        value={currentInstructions}
                        onChange={(e) => setInstructionsDraft(e.target.value)}
                        placeholder="e.g. I prefer concise code answers with TypeScript and modern standards..."
                        rows={4}
                        className={cn(
                          "w-full rounded-xl border border-input bg-transparent p-3 text-xs outline-none transition-colors",
                          "focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/40",
                          instructionsTooLong && "border-destructive focus-visible:border-destructive"
                        )}
                      />
                      <div className="flex items-center justify-between">
                        <span
                          className={cn(
                            "text-[11px]",
                            instructionsTooLong ? "text-destructive font-medium" : "text-muted-foreground"
                          )}
                        >
                          {currentInstructions.length} / {MAX_INSTRUCTIONS_CHARS}
                        </span>
                        <Button
                          size="sm"
                          disabled={!dirtyInstructions || instructionsTooLong || updateProfile.isPending}
                          onClick={saveInstructions}
                        >
                          {updateProfile.isPending ? "Saving…" : "Save instructions"}
                        </Button>
                      </div>
                    </div>
                  )}
                </div>

                {/* Cross-chat Memories */}
                <div className="border-t pt-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <div className="flex items-center gap-1.5">
                        <Brain className="size-4 text-primary" />
                        <h3 className="text-sm font-semibold text-foreground">Saved memories</h3>
                      </div>
                      <p className="text-xs text-muted-foreground mt-0.5">
                        Details Herbie has remembered across your conversations.
                      </p>
                    </div>
                    {memories && memories.length > 0 && (
                      <Button
                        variant="ghost"
                        size="xs"
                        className="text-destructive hover:text-destructive text-xs"
                        disabled={clearMemories.isPending}
                        onClick={() => {
                          if (confirm("Clear all remembered memories?")) {
                            clearMemories.mutate(undefined, {
                              onSuccess: () => toast.success("Memories cleared"),
                            });
                          }
                        }}
                      >
                        Clear all
                      </Button>
                    )}
                  </div>

                  <form onSubmit={handleAddMemory} className="mt-3 flex gap-2">
                    <input
                      aria-label="New memory"
                      value={newMemory}
                      onChange={(e) => setNewMemory(e.target.value)}
                      placeholder="Add something for Herbie to remember…"
                      className="flex-1 rounded-lg border border-input bg-transparent px-3 py-1.5 text-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/40"
                    />
                    <Button
                      type="submit"
                      size="sm"
                      disabled={!newMemory.trim() || createMemory.isPending}
                    >
                      <Plus className="size-3.5" /> Add
                    </Button>
                  </form>

                  <div className="mt-3 max-h-44 space-y-1.5 overflow-y-auto">
                    {memoriesLoading && <Skeleton className="h-10 w-full rounded-lg" />}
                    {!memoriesLoading && (!memories || memories.length === 0) && (
                      <p className="py-4 text-center text-xs text-muted-foreground">
                        No memories yet. You can tell Herbie to remember things, or add them here.
                      </p>
                    )}
                    {memories?.map((m) => (
                      <div
                        key={m.id}
                        className="group flex items-center justify-between gap-2 rounded-lg border border-border/50 bg-card px-3 py-2 text-xs"
                      >
                        <span className="min-w-0 flex-1 truncate">{m.content}</span>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          className="opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-destructive"
                          onClick={() => deleteMemory.mutate(m.id)}
                          aria-label={`Delete memory: ${m.content}`}
                        >
                          <Trash2 className="size-3" />
                        </Button>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            )}

            {activeTab === "tools" && (
              <div className="space-y-5">
                {/* Header & Quick stats */}
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 border-b border-border/60">
                  <div>
                    <h3 className="text-sm font-semibold text-foreground">Tools, Apps & Integrations</h3>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      Services, web APIs, and external protocols Herbie can use autonomously during chat.
                    </p>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Button variant="outline" size="sm" asChild onClick={() => onOpenChange(false)}>
                      <Link to="/tools" className="text-xs">Tools Gallery</Link>
                    </Button>
                    <Button variant="outline" size="sm" asChild onClick={() => onOpenChange(false)}>
                      <Link to="/workflows" className="text-xs">Workflows</Link>
                    </Button>
                  </div>
                </div>

                {/* Status overview cards */}
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
                  <div className="rounded-xl border border-border bg-card p-2.5 text-xs shadow-2xs">
                    <span className="text-[11px] text-muted-foreground">HTTP Tools</span>
                    <p className="text-lg font-bold text-foreground mt-0.5">{tools?.length ?? 0}</p>
                    <span className="text-[10px] text-muted-foreground">{tools?.filter(t => t.enabled).length ?? 0} active</span>
                  </div>
                  <div className="rounded-xl border border-border bg-card p-2.5 text-xs shadow-2xs">
                    <span className="text-[11px] text-muted-foreground">Chat Apps</span>
                    <p className="text-lg font-bold text-foreground mt-0.5">
                      {(oauthProviders?.filter(p => p.connected).length ?? 0) + 1}
                    </p>
                    <span className="text-[10px] text-emerald-600 dark:text-emerald-400">OAuth & Built-in</span>
                  </div>
                  <div className="rounded-xl border border-border bg-card p-2.5 text-xs shadow-2xs">
                    <span className="text-[11px] text-muted-foreground">MCP Servers</span>
                    <p className="text-lg font-bold text-foreground mt-0.5">
                      {mcpServers?.filter(s => s.enabled).length ?? 0}
                    </p>
                    <span className="text-[10px] text-purple-600 dark:text-purple-400">JSON-RPC 2.0</span>
                  </div>
                  <div className="rounded-xl border border-border bg-card p-2.5 text-xs shadow-2xs">
                    <span className="text-[11px] text-muted-foreground">Workflows</span>
                    <p className="text-lg font-bold text-foreground mt-0.5">
                      {workflows?.filter(w => w.exposeAsTool).length ?? 0}
                    </p>
                    <span className="text-[10px] text-muted-foreground">Exposed to Chat</span>
                  </div>
                </div>

                {/* 1-Click Apps & OAuth Section */}
                <div className="space-y-2.5">
                  <div className="flex items-center justify-between">
                    <div>
                      <span className="text-xs font-semibold text-foreground">Integrated Apps & MCP Tools (@ mentions)</span>
                      <p className="text-[11px] text-muted-foreground">1-Click connect curated integrations to your AI assistant.</p>
                    </div>
                    <Button
                      size="xs"
                      variant="outline"
                      type="button"
                      onClick={() => openLinkApp("github")}
                      className="text-xs gap-1 border-purple-500/30 text-purple-600 dark:text-purple-400 hover:bg-purple-500/10"
                    >
                      <Sparkles className="size-3" />
                      <span>MCP Directory</span>
                    </Button>
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                    {/* Google Calendar */}
                    {(() => {
                      const cal = oauthProviders?.find(p => p.id === "google_calendar");
                      const isConnected = !!cal?.connected;
                      return (
                        <div className="flex items-center justify-between rounded-xl border border-border bg-card p-2.5 text-xs shadow-2xs">
                          <div className="flex items-center gap-2.5 min-w-0">
                            <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-sky-500/10 text-sky-600 dark:text-sky-400">
                              <Calendar className="size-3.5" />
                            </div>
                            <div className="min-w-0">
                              <div className="flex items-center gap-1.5">
                                <span className="font-semibold text-foreground truncate">Google Calendar</span>
                                <span className="font-mono text-[10px] text-muted-foreground">@calendar</span>
                              </div>
                              <p className="text-[10px] text-muted-foreground truncate">
                                {isConnected
                                  ? (cal?.connectedVia === "mcp" ? "Linked via MCP Server" : "Connected via OAuth")
                                  : "Agenda & event creation"}
                              </p>
                            </div>
                          </div>
                          <div className="flex items-center gap-1.5 shrink-0">
                            <Badge variant="outline" className={cn("text-[10px] gap-1 px-1.5 py-0 font-normal shrink-0", isConnected ? "border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400" : "text-muted-foreground")}>
                              {isConnected && <CheckCircle2 className="size-2.5" />}
                              <span>{isConnected ? (cal?.connectedVia === "mcp" ? "MCP" : "OAuth") : "Not Linked"}</span>
                            </Badge>
                            <Button
                              size="xs"
                              variant={isConnected ? "ghost" : "default"}
                              type="button"
                              onClick={() => isConnected ? unlinkCatalogApp.mutate("google_calendar") : linkCatalogApp.mutate({ appId: "google_calendar" })}
                              className={cn("h-6 px-2 text-[10px]", isConnected ? "text-destructive hover:bg-destructive/10" : "bg-purple-600 hover:bg-purple-700 text-white")}
                            >
                              {isConnected ? "Unlink" : "1-Click Link"}
                            </Button>
                          </div>
                        </div>
                      );
                    })()}

                    {/* GitHub */}
                    {(() => {
                      const gh = oauthProviders?.find(p => p.id === "github");
                      const isConnected = !!gh?.connected;
                      return (
                        <div className="flex items-center justify-between rounded-xl border border-border bg-card p-2.5 text-xs shadow-2xs">
                          <div className="flex items-center gap-2.5 min-w-0">
                            <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-muted text-foreground">
                              <GitBranch className="size-3.5" />
                            </div>
                            <div className="min-w-0">
                              <div className="flex items-center gap-1.5">
                                <span className="font-semibold text-foreground truncate">GitHub</span>
                                <span className="font-mono text-[10px] text-muted-foreground">@github</span>
                              </div>
                              <p className="text-[10px] text-muted-foreground truncate">
                                {isConnected
                                  ? (gh?.connectedVia === "mcp" ? "Linked via MCP Server" : "Connected via OAuth")
                                  : "Repos, PRs & issues"}
                              </p>
                            </div>
                          </div>
                          <div className="flex items-center gap-1.5 shrink-0">
                            <Badge variant="outline" className={cn("text-[10px] gap-1 px-1.5 py-0 font-normal shrink-0", isConnected ? "border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400" : "text-muted-foreground")}>
                              {isConnected && <CheckCircle2 className="size-2.5" />}
                              <span>{isConnected ? (gh?.connectedVia === "mcp" ? "MCP" : "OAuth") : "Not Linked"}</span>
                            </Badge>
                            <Button
                              size="xs"
                              variant={isConnected ? "ghost" : "default"}
                              type="button"
                              onClick={() => isConnected ? unlinkCatalogApp.mutate("github") : linkCatalogApp.mutate({ appId: "github" })}
                              className={cn("h-6 px-2 text-[10px]", isConnected ? "text-destructive hover:bg-destructive/10" : "bg-purple-600 hover:bg-purple-700 text-white")}
                            >
                              {isConnected ? "Unlink" : "1-Click Link"}
                            </Button>
                          </div>
                        </div>
                      );
                    })()}

                    {/* Slack */}
                    {(() => {
                      const slk = oauthProviders?.find(p => p.id === "slack");
                      const isConnected = !!slk?.connected;
                      return (
                        <div className="flex items-center justify-between rounded-xl border border-border bg-card p-2.5 text-xs shadow-2xs">
                          <div className="flex items-center gap-2.5 min-w-0">
                            <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                              <MessageSquare className="size-3.5" />
                            </div>
                            <div className="min-w-0">
                              <div className="flex items-center gap-1.5">
                                <span className="font-semibold text-foreground truncate">Slack</span>
                                <span className="font-mono text-[10px] text-muted-foreground">@slack</span>
                              </div>
                              <p className="text-[10px] text-muted-foreground truncate">
                                {isConnected
                                  ? (slk?.connectedVia === "mcp" ? "Linked via MCP Server" : "Connected via OAuth")
                                  : "Channel notifications"}
                              </p>
                            </div>
                          </div>
                          <div className="flex items-center gap-1.5 shrink-0">
                            <Badge variant="outline" className={cn("text-[10px] gap-1 px-1.5 py-0 font-normal shrink-0", isConnected ? "border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400" : "text-muted-foreground")}>
                              {isConnected && <CheckCircle2 className="size-2.5" />}
                              <span>{isConnected ? (slk?.connectedVia === "mcp" ? "MCP" : "OAuth") : "Not Linked"}</span>
                            </Badge>
                            <Button
                              size="xs"
                              variant={isConnected ? "ghost" : "default"}
                              type="button"
                              onClick={() => isConnected ? unlinkCatalogApp.mutate("slack") : linkCatalogApp.mutate({ appId: "slack" })}
                              className={cn("h-6 px-2 text-[10px]", isConnected ? "text-destructive hover:bg-destructive/10" : "bg-purple-600 hover:bg-purple-700 text-white")}
                            >
                              {isConnected ? "Unlink" : "1-Click Link"}
                            </Button>
                          </div>
                        </div>
                      );
                    })()}

                    {/* Web Search */}
                    <div className="flex items-center justify-between rounded-xl border border-border bg-card p-2.5 text-xs shadow-2xs">
                      <div className="flex items-center gap-2.5 min-w-0">
                        <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-cyan-500/10 text-cyan-600 dark:text-cyan-400">
                          <Globe className="size-3.5" />
                        </div>
                        <div className="min-w-0">
                          <div className="flex items-center gap-1.5">
                            <span className="font-semibold text-foreground truncate">Web Search</span>
                            <span className="font-mono text-[10px] text-muted-foreground">@web</span>
                          </div>
                          <p className="text-[10px] text-muted-foreground truncate">Live public web retrieval</p>
                        </div>
                      </div>
                      <Badge variant="outline" className="border-cyan-600/30 bg-cyan-600/10 text-cyan-700 dark:text-cyan-400 text-[10px] px-1.5 py-0 font-normal shrink-0">
                        Built-in
                      </Badge>
                    </div>

                    {/* Web Reader & Fetch */}
                    {(() => {
                      const isConnected = mcpServers?.some((s) => s.appId === "web_fetch" && s.enabled);
                      return (
                        <div className="flex items-center justify-between rounded-xl border border-border bg-card p-2.5 text-xs shadow-2xs">
                          <div className="flex items-center gap-2.5 min-w-0">
                            <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-blue-500/10 text-blue-600 dark:text-blue-400">
                              <Globe className="size-3.5" />
                            </div>
                            <div className="min-w-0">
                              <div className="flex items-center gap-1.5">
                                <span className="font-semibold text-foreground truncate">Web Reader</span>
                                <span className="font-mono text-[10px] text-muted-foreground">@web_fetch</span>
                              </div>
                              <p className="text-[10px] text-muted-foreground truncate">Article text extraction</p>
                            </div>
                          </div>
                          <div className="flex items-center gap-1.5 shrink-0">
                            <Badge variant="outline" className={cn("text-[10px] gap-1 px-1.5 py-0 font-normal shrink-0", isConnected ? "border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400" : "text-muted-foreground")}>
                              {isConnected && <CheckCircle2 className="size-2.5" />}
                              <span>{isConnected ? "MCP" : "Not Linked"}</span>
                            </Badge>
                            <Button
                              size="xs"
                              variant={isConnected ? "ghost" : "default"}
                              type="button"
                              onClick={() => isConnected ? unlinkCatalogApp.mutate("web_fetch") : linkCatalogApp.mutate({ appId: "web_fetch" })}
                              className={cn("h-6 px-2 text-[10px]", isConnected ? "text-destructive hover:bg-destructive/10" : "bg-purple-600 hover:bg-purple-700 text-white")}
                            >
                              {isConnected ? "Unlink" : "1-Click Link"}
                            </Button>
                          </div>
                        </div>
                      );
                    })()}

                    {/* Code Sandbox */}
                    {(() => {
                      const isConnected = mcpServers?.some((s) => s.appId === "code_runner" && s.enabled);
                      return (
                        <div className="flex items-center justify-between rounded-xl border border-border bg-card p-2.5 text-xs shadow-2xs">
                          <div className="flex items-center gap-2.5 min-w-0">
                            <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-amber-500/10 text-amber-600 dark:text-amber-400">
                              <Terminal className="size-3.5" />
                            </div>
                            <div className="min-w-0">
                              <div className="flex items-center gap-1.5">
                                <span className="font-semibold text-foreground truncate">Code Sandbox</span>
                                <span className="font-mono text-[10px] text-muted-foreground">@code_runner</span>
                              </div>
                              <p className="text-[10px] text-muted-foreground truncate">Sandbox calculations</p>
                            </div>
                          </div>
                          <div className="flex items-center gap-1.5 shrink-0">
                            <Badge variant="outline" className={cn("text-[10px] gap-1 px-1.5 py-0 font-normal shrink-0", isConnected ? "border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400" : "text-muted-foreground")}>
                              {isConnected && <CheckCircle2 className="size-2.5" />}
                              <span>{isConnected ? "MCP" : "Not Linked"}</span>
                            </Badge>
                            <Button
                              size="xs"
                              variant={isConnected ? "ghost" : "default"}
                              type="button"
                              onClick={() => isConnected ? unlinkCatalogApp.mutate("code_runner") : linkCatalogApp.mutate({ appId: "code_runner" })}
                              className={cn("h-6 px-2 text-[10px]", isConnected ? "text-destructive hover:bg-destructive/10" : "bg-purple-600 hover:bg-purple-700 text-white")}
                            >
                              {isConnected ? "Unlink" : "1-Click Link"}
                            </Button>
                          </div>
                        </div>
                      );
                    })()}
                  </div>
                </div>

                {/* Configured Custom HTTP Tools */}
                <div className="space-y-2.5 pt-2 border-t border-border/60">
                  <div className="flex items-center justify-between">
                    <div>
                      <span className="text-xs font-semibold text-foreground">Configured HTTP Tools</span>
                      <p className="text-[11px] text-muted-foreground">Custom REST & Web API endpoints connected to Herbie.</p>
                    </div>
                    <Button variant="ghost" size="xs" asChild onClick={() => onOpenChange(false)}>
                      <Link to="/tools" className="text-[11px] text-primary gap-1">
                        <span>Add new tool</span>
                        <Plus className="size-3" />
                      </Link>
                    </Button>
                  </div>

                  {!tools || tools.length === 0 ? (
                    <p className="text-xs text-muted-foreground py-3 text-center border rounded-xl border-dashed">
                      No custom HTTP tools configured yet. Browse templates in the Tools gallery.
                    </p>
                  ) : (
                    <div className="space-y-2">
                      {tools.map((tool) => (
                        <div
                          key={tool.id}
                          className="flex flex-col gap-2 rounded-xl border border-border/70 bg-card p-3 text-xs shadow-2xs"
                        >
                          <div className="flex items-start justify-between gap-2">
                            <div className="min-w-0 flex-1">
                              <div className="flex flex-wrap items-center gap-2">
                                <p className="font-semibold text-foreground text-xs">{tool.name}</p>
                                <span className="font-mono text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-semibold">
                                  {tool.method}
                                </span>
                                {tool.requireApproval && (
                                  <Badge variant="outline" className="text-[10px] border-amber-500/30 text-amber-600 px-1 py-0 gap-1 font-normal">
                                    <ShieldCheck className="size-2.5" />
                                    <span>Approval Gated</span>
                                  </Badge>
                                )}
                              </div>
                              <p className="text-muted-foreground text-xs mt-1 leading-relaxed">{tool.description}</p>
                            </div>

                            <Button
                              size="xs"
                              variant={tool.enabled ? "outline" : "ghost"}
                              onClick={() => updateTool.mutate({ id: tool.id, patch: { enabled: !tool.enabled } })}
                              className={cn(
                                "h-6 text-[10px] px-2 gap-1 font-medium shrink-0",
                                tool.enabled
                                  ? "text-emerald-600 border-emerald-600/30 bg-emerald-500/5 hover:bg-emerald-500/10"
                                  : "text-muted-foreground"
                              )}
                            >
                              <Power className="size-2.5" />
                              <span>{tool.enabled ? "Enabled" : "Disabled"}</span>
                            </Button>
                          </div>

                          <div className="flex items-center gap-1 text-[11px] font-mono text-muted-foreground bg-muted/40 px-2 py-1 rounded-md overflow-hidden">
                            <span className="text-foreground/70 font-semibold">{tool.method}</span>
                            <span className="truncate">{tool.urlTemplate}</span>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* Model Context Protocol (MCP) Information Banner */}
                <div className="rounded-xl border border-purple-500/30 bg-purple-500/5 p-3 text-xs space-y-1.5">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Plug className="size-4 text-purple-600 dark:text-purple-400" />
                      <span className="font-semibold text-foreground">Model Context Protocol (MCP)</span>
                    </div>
                    <Badge variant="outline" className="border-purple-500/40 text-purple-700 dark:text-purple-300 text-[10px] font-normal">
                      Active
                    </Badge>
                  </div>
                  <p className="text-[11px] text-muted-foreground leading-relaxed">
                    Herbie connects to external MCP servers and provides its own MCP endpoint at <code className="font-mono text-foreground font-semibold">POST /api/mcp</code> for clients like Cursor and Claude Desktop.
                  </p>
                </div>
              </div>
            )}

            {activeTab === "documents" && (
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <h3 className="text-sm font-semibold text-foreground">Knowledge Base</h3>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      Upload and index files for hybrid RAG search and citation.
                    </p>
                  </div>
                  <Button variant="outline" size="sm" asChild onClick={() => onOpenChange(false)}>
                    <Link to="/documents">Open Documents</Link>
                  </Button>
                </div>

                <div className="rounded-xl border border-border/60 bg-muted/10 p-3">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">Indexed documents</span>
                    <span className="font-semibold">{docs?.length ?? 0}</span>
                  </div>
                </div>

                <div className="space-y-2">
                  {docs?.slice(0, 5).map((doc) => (
                    <div
                      key={doc.id}
                      className="flex items-center justify-between rounded-lg border border-border/50 p-2.5 text-xs"
                    >
                      <div className="min-w-0 flex-1">
                        <p className="font-medium truncate">{doc.filename}</p>
                        <p className="text-muted-foreground text-[11px]">{doc.chunkCount} chunks indexed</p>
                      </div>
                      <span className="rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
                        {doc.status}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {activeTab === "account" && (
              <div className="space-y-6">
                <div>
                  <h3 className="text-sm font-semibold text-foreground">User Profile</h3>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    Account credentials and session information.
                  </p>
                  <div className="mt-3 rounded-xl border border-border/60 bg-muted/10 p-4 space-y-2 text-xs">
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Email</span>
                      <span className="font-medium">{user?.email ?? "Not logged in"}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">User ID</span>
                      <span className="font-mono text-[11px] text-muted-foreground truncate max-w-[200px]">
                        {user?.id ?? "—"}
                      </span>
                    </div>
                  </div>
                </div>

                <div className="border-t pt-4 flex items-center justify-between">
                  <div>
                    <h4 className="text-xs font-semibold">Usage & Ledger</h4>
                    <p className="text-[11px] text-muted-foreground">Check metered tokens and costs.</p>
                  </div>
                  <Button variant="outline" size="sm" asChild onClick={() => onOpenChange(false)}>
                    <Link to="/usage">
                      <Gauge className="size-3.5" /> View usage
                    </Link>
                  </Button>
                </div>

                <div className="border-t pt-4 flex justify-end">
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => {
                      onOpenChange(false);
                      void handleLogout();
                    }}
                  >
                    Log out
                  </Button>
                </div>
              </div>
            )}
          </div>
        </div>
      </DialogContent>

      <LinkAppDialog
        open={linkAppDialogOpen}
        onOpenChange={setLinkAppDialogOpen}
        initialAppId={linkAppInitialId}
      />
    </Dialog>
  );
}
