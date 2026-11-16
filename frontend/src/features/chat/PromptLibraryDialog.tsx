import { useState } from "react";
import { BookMarked, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { ApiError } from "@/lib/api";
import type { SavedPrompt } from "@/lib/types";
import {
  useCreateSavedPrompt,
  useDeleteSavedPrompt,
  useSavedPrompts,
  useUpdateSavedPrompt,
} from "@/features/chat/useConversations";

const MAX_TITLE = 200;
const MAX_BODY = 8000;

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onInsert: (body: string) => void;
}

interface PromptDraft {
  id?: string;
  title: string;
  body: string;
}

function errorMessage(err: unknown, fallback: string) {
  return err instanceof ApiError ? err.message : fallback;
}

/** Reusable composer snippets: list, create/edit, delete, and insert at the
 *  caller's caret. Variables are out of scope for the first version. */
export function PromptLibraryDialog({ open, onOpenChange, onInsert }: Props) {
  const { data: prompts, isLoading } = useSavedPrompts(open);
  const createPrompt = useCreateSavedPrompt();
  const updatePrompt = useUpdateSavedPrompt();
  const deletePrompt = useDeleteSavedPrompt();
  const [draft, setDraft] = useState<PromptDraft | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  const saving = createPrompt.isPending || updatePrompt.isPending;
  const canSave = Boolean(draft && draft.title.trim() && draft.body.trim());

  function startCreate() {
    setConfirmDelete(null);
    setDraft({ title: "", body: "" });
  }

  function startEdit(prompt: SavedPrompt) {
    setConfirmDelete(null);
    setDraft({ id: prompt.id, title: prompt.title, body: prompt.body });
  }

  function save() {
    if (!draft || !canSave) return;
    const input = { title: draft.title.trim(), body: draft.body };
    const done = () => {
      setDraft(null);
      toast.success(draft.id ? "Prompt updated" : "Prompt saved");
    };
    if (draft.id) {
      updatePrompt.mutate({ id: draft.id, ...input }, { onSuccess: done, onError: (err) => toast.error(errorMessage(err, "Failed to update prompt")) });
    } else {
      createPrompt.mutate(input, { onSuccess: done, onError: (err) => toast.error(errorMessage(err, "Failed to save prompt")) });
    }
  }

  function remove(id: string) {
    deletePrompt.mutate(id, {
      onSuccess: () => {
        setConfirmDelete(null);
        if (draft?.id === id) setDraft(null);
        toast.success("Prompt deleted");
      },
      onError: (err) => toast.error(errorMessage(err, "Failed to delete prompt")),
    });
  }

  function insert(prompt: SavedPrompt) {
    onInsert(prompt.body);
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <BookMarked className="size-4 text-primary" />
            Prompt library
          </DialogTitle>
          <DialogDescription>Save reusable prompts and insert them into the composer.</DialogDescription>
        </DialogHeader>

        {draft ? (
          <div className="grid gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="prompt-title">Title</Label>
              <Input
                id="prompt-title"
                value={draft.title}
                maxLength={MAX_TITLE}
                onChange={(e) => setDraft({ ...draft, title: e.target.value })}
                placeholder="e.g. Weekly summary"
              />
            </div>
            <div className="grid gap-1.5">
              <div className="flex items-center justify-between">
                <Label htmlFor="prompt-body">Prompt</Label>
                <span className="text-[11px] text-muted-foreground">
                  {draft.body.length}/{MAX_BODY}
                </span>
              </div>
              <textarea
                id="prompt-body"
                value={draft.body}
                maxLength={MAX_BODY}
                rows={6}
                onChange={(e) => setDraft({ ...draft, body: e.target.value })}
                placeholder="Write the prompt text…"
                className="max-h-72 min-h-24 w-full resize-none rounded-lg border border-input bg-transparent px-3 py-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/20"
              />
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setDraft(null)}>Cancel</Button>
              <Button onClick={save} disabled={!canSave || saving}>{saving ? "Saving…" : "Save"}</Button>
            </div>
          </div>
        ) : (
          <div className="grid gap-3">
            <div className="flex justify-end">
              <Button size="sm" variant="outline" onClick={startCreate}>
                <Plus className="size-3.5" />
                New prompt
              </Button>
            </div>
            <ScrollArea className="max-h-80">
              {isLoading ? (
                <p className="py-8 text-center text-sm text-muted-foreground">Loading prompts…</p>
              ) : (prompts?.length ?? 0) === 0 ? (
                <div className="py-10 text-center text-sm text-muted-foreground">
                  <BookMarked className="mx-auto mb-2 size-6 opacity-50" />
                  No saved prompts yet. Save one to reuse it later.
                </div>
              ) : (
                <ul className="space-y-2">
                  {prompts?.map((prompt) => (
                    <li key={prompt.id} className="rounded-xl border border-border/70 p-3">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium">{prompt.title}</p>
                          <p className="mt-0.5 line-clamp-2 whitespace-pre-wrap text-xs text-muted-foreground">
                            {prompt.body}
                          </p>
                        </div>
                        <div className="flex shrink-0 items-center gap-1">
                          <Button size="xs" variant="brand" onClick={() => insert(prompt)}>Insert</Button>
                          <Button
                            size="icon-xs"
                            variant="ghost"
                            aria-label={`Edit ${prompt.title}`}
                            title="Edit prompt"
                            onClick={() => startEdit(prompt)}
                          >
                            <Pencil className="size-3.5" />
                          </Button>
                          <Button
                            size="icon-xs"
                            variant="ghost"
                            aria-label={`Delete ${prompt.title}`}
                            title="Delete prompt"
                            className="text-muted-foreground hover:text-destructive"
                            onClick={() => setConfirmDelete(prompt.id)}
                          >
                            <Trash2 className="size-3.5" />
                          </Button>
                        </div>
                      </div>
                      {confirmDelete === prompt.id && (
                        <div className="mt-2 flex items-center justify-end gap-2 border-t border-border/60 pt-2 text-xs">
                          <span className="text-muted-foreground">Delete “{prompt.title}”?</span>
                          <Button size="xs" variant="outline" onClick={() => setConfirmDelete(null)}>Cancel</Button>
                          <Button size="xs" variant="destructive" onClick={() => remove(prompt.id)} disabled={deletePrompt.isPending}>
                            {deletePrompt.isPending ? "Deleting…" : "Delete"}
                          </Button>
                        </div>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </ScrollArea>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
