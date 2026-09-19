import React, { useState } from "react";
import { toast } from "sonner";
import { Brain, Plus, Sidebar, Sliders, Trash2 } from "lucide-react";
import { ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useSidebar } from "@/components/layout/SidebarContext";
import { useProfile, useUpdateProfile } from "@/features/profile/useProfile";
import {
  useMemories,
  useCreateMemory,
  useDeleteMemory,
  useClearMemories,
} from "@/features/memories/useMemories";

export const MAX_INSTRUCTIONS_CHARS = 4000;
export const MAX_MEMORY_CHARS = 1000;

export function PreferencesPage() {
  const { toggleSidebar, setMobileOpen } = useSidebar();
  const { data, isLoading, isError, refetch, isFetching } = useProfile();
  const update = useUpdateProfile();
  const [draft, setDraft] = useState<string | null>(null);

  const {
    data: memories,
    isLoading: memoriesLoading,
    isError: memoriesError,
    refetch: refetchMemories,
  } = useMemories();
  const createMemory = useCreateMemory();
  const deleteMemory = useDeleteMemory();
  const clearMemories = useClearMemories();

  const [newMemory, setNewMemory] = useState("");

  const currentDraft = draft !== null ? draft : (data?.defaultInstructions ?? "");
  const dirty = draft !== null && data !== undefined && draft !== data.defaultInstructions;
  const tooLong = currentDraft.length > MAX_INSTRUCTIONS_CHARS;

  function saveInstructions() {
    if (draft === null) return;
    update.mutate(draft, {
      onSuccess: () => toast.success("Preferences saved"),
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

  function handleDeleteMemory(id: string) {
    deleteMemory.mutate(id, {
      onSuccess: () => toast.success("Memory deleted"),
      onError: (err) =>
        toast.error(err instanceof ApiError ? err.message : "Failed to delete memory"),
    });
  }

  function handleClearAll() {
    if (!confirm("Are you sure you want to clear all saved memories?")) return;
    clearMemories.mutate(undefined, {
      onSuccess: () => toast.success("All memories cleared"),
      onError: (err) =>
        toast.error(err instanceof ApiError ? err.message : "Failed to clear memories"),
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
            <Sliders className="size-4 text-muted-foreground" />
            <h1 className="text-sm font-semibold tracking-tight">Preferences</h1>
          </div>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-2xl space-y-6 p-6">

      {isLoading && <Skeleton className="h-64 w-full" />}

      {isError && (
        <div className="flex items-center gap-3">
          <p className="text-destructive text-sm">Failed to load preferences.</p>
          <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
            Retry
          </Button>
        </div>
      )}

      {data && (
        <Card>
          <CardHeader>
            <CardTitle>Custom instructions</CardTitle>
            <CardDescription>
              Applied to every conversation before its own system prompt, which wins ties.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <textarea
              value={currentDraft}
              onChange={(e) => setDraft(e.target.value)}
              rows={8}
              maxLength={MAX_INSTRUCTIONS_CHARS + 100}
              placeholder="e.g. Always answer concisely. Prefer TypeScript examples."
              aria-label="Custom instructions"
              className="min-h-40 w-full rounded-md border border-input bg-transparent p-3 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
            />
            <div className="flex items-center justify-between">
              <p className={tooLong ? "text-destructive text-xs" : "text-muted-foreground text-xs"}>
                {`${currentDraft.length} / ${MAX_INSTRUCTIONS_CHARS}`}
              </p>
              <Button size="sm" variant="brand" disabled={!dirty || tooLong || update.isPending} onClick={saveInstructions}>
                {update.isPending ? "Saving…" : "Save"}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Cross-chat Memory Section */}
      <Card>
        <CardHeader className="flex flex-row items-start justify-between">
          <div>
            <div className="flex items-center gap-2">
              <Brain className="size-4 text-primary" />
              <CardTitle>Memory</CardTitle>
            </div>
            <CardDescription className="mt-1">
              Enduring facts and preferences remembered about you across all conversations.
              The assistant can also save memories automatically when asked.
            </CardDescription>
          </div>
          {memories && memories.length > 0 && (
            <Button
              variant="outline"
              size="sm"
              onClick={handleClearAll}
              disabled={clearMemories.isPending}
              className="text-xs text-destructive hover:bg-destructive/10 cursor-pointer"
            >
              Clear all
            </Button>
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          <form onSubmit={handleAddMemory} className="flex gap-2">
            <input
              type="text"
              value={newMemory}
              onChange={(e) => setNewMemory(e.target.value)}
              placeholder="Add a memory (e.g. 'I work with Go and React')"
              aria-label="New memory"
              maxLength={MAX_MEMORY_CHARS}
              className="flex-1 rounded-md border border-input bg-transparent px-3 py-1.5 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
            />
            <Button
              type="submit"
              size="sm"
              disabled={!newMemory.trim() || createMemory.isPending}
              className="flex items-center gap-1.5 cursor-pointer"
            >
              <Plus className="size-3.5" />
              <span>Add</span>
            </Button>
          </form>

          {memoriesLoading && <Skeleton className="h-28 w-full" />}

          {memoriesError && (
            <div className="flex items-center gap-3">
              <p className="text-destructive text-sm">Failed to load memories.</p>
              <Button variant="outline" size="sm" onClick={() => void refetchMemories()}>
                Retry
              </Button>
            </div>
          )}

          {memories && memories.length === 0 && (
            <div className="rounded-md border border-dashed p-6 text-center text-xs text-muted-foreground">
              No memories saved yet. You can add facts here or ask the assistant to remember things in chat.
            </div>
          )}

          {memories && memories.length > 0 && (
            <ul className="divide-y rounded-md border bg-muted/10">
              {memories.map((m) => (
                <li
                  key={m.id}
                  className="flex items-center justify-between p-3 text-sm hover:bg-muted/20 transition-colors"
                >
                  <div className="flex-1 pr-4">
                    <p className="font-medium text-foreground">{m.content}</p>
                    <p className="text-[11px] text-muted-foreground mt-0.5">
                      {new Date(m.createdAt).toLocaleDateString()}
                    </p>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => handleDeleteMemory(m.id)}
                    aria-label={`Delete memory: ${m.content}`}
                    title="Delete memory"
                    disabled={deleteMemory.isPending}
                    className="text-muted-foreground hover:text-destructive cursor-pointer"
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
        </div>
      </div>
    </div>
  );
}
