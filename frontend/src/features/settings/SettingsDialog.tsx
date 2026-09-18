import React, { useState, useEffect } from "react";
import { Link } from "react-router";
import { useTheme } from "next-themes";
import { toast } from "sonner";
import {
  Brain,
  FileText,
  Gauge,
  Laptop,
  Moon,
  Plus,
  Settings,
  Sparkles,
  Sun,
  Trash2,
  User,
  Wrench,
  X,
} from "lucide-react";
import { ApiError, apiFetch } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useAuth } from "@/stores/auth";
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
import { useTools } from "@/features/tools/useTools";
import { useDocuments } from "@/features/documents/useDocuments";

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

  // Tools & Docs counts
  const { data: tools } = useTools();
  const { data: docs } = useDocuments();

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
    { id: "personalization", label: "Personalization", icon: Sparkles },
    { id: "tools", label: "Tools", icon: Wrench },
    { id: "documents", label: "Documents", icon: FileText },
    { id: "account", label: "Account", icon: User },
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="sm:max-w-2xl p-0 overflow-hidden rounded-2xl gap-0">
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

        <div className="flex flex-col md:flex-row min-h-[420px] max-h-[80vh]">
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
          <div className="flex-1 overflow-y-auto p-5">
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
                        What would you like Golem to know about you to provide better responses?
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
                        Details Golem has remembered across your conversations.
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
                      placeholder="Add something for Golem to remember…"
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
                        No memories yet. You can tell Golem to remember things, or add them here.
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
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <h3 className="text-sm font-semibold text-foreground">HTTP Tools & APIs</h3>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      Enable external web APIs and services for Golem to call during chat.
                    </p>
                  </div>
                  <Button variant="outline" size="sm" asChild onClick={() => onOpenChange(false)}>
                    <Link to="/tools">Open Tools Gallery</Link>
                  </Button>
                </div>

                <div className="rounded-xl border border-border/60 bg-muted/10 p-3">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">Configured tools</span>
                    <span className="font-semibold">{tools?.length ?? 0}</span>
                  </div>
                </div>

                <div className="space-y-2">
                  {tools?.slice(0, 5).map((tool) => (
                    <div
                      key={tool.id}
                      className="flex items-center justify-between rounded-lg border border-border/50 p-2.5 text-xs"
                    >
                      <div className="min-w-0 flex-1">
                        <p className="font-medium truncate">{tool.name}</p>
                        <p className="text-muted-foreground text-[11px] truncate">{tool.description}</p>
                      </div>
                      <span className={cn(
                        "rounded-full px-2 py-0.5 text-[10px] font-medium",
                        tool.enabled ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" : "bg-muted text-muted-foreground"
                      )}>
                        {tool.enabled ? "Active" : "Disabled"}
                      </span>
                    </div>
                  ))}
                  {(tools?.length ?? 0) > 5 && (
                    <p className="text-center text-xs text-muted-foreground pt-1">
                      +{(tools?.length ?? 0) - 5} more tools. Manage in Tools page.
                    </p>
                  )}
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
    </Dialog>
  );
}
