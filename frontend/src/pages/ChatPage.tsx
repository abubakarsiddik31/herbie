import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { LogOut, Pencil, Plus, Send, Square, Trash2 } from "lucide-react";
import { ApiError, apiFetch } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { ChatMessage, Conversation } from "@/lib/types";
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
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { RunLoader } from "@/components/ai/RunLoader";
import { StreamingText } from "@/components/ai/StreamingText";
import { ThinkingTrace } from "@/components/ai/ThinkingTrace";
import { useChat } from "@/features/chat/useChat";
import {
  useConversations,
  useCreateConversation,
  useDeleteConversation,
} from "@/features/chat/useConversations";

interface ConversationDetail {
  conversation: Conversation;
  messages: ChatMessage[];
}

// Server messages arrive without the ephemeral streaming/error fields.
function toChatMessage(m: ChatMessage): ChatMessage {
  return { id: m.id, role: m.role, content: m.content, truncated: m.truncated, createdAt: m.createdAt };
}

export function ChatPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [input, setInput] = useState("");
  const [sendError, setSendError] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Conversation | null>(null);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const seededRef = useRef<string | null>(null);

  const { messages, setMessages, status, send, stop, reset, trace } = useChat(() => {
    // Refresh titles/order after each completed run.
    void queryClient.invalidateQueries({ queryKey: ["conversations"] });
  });

  const { data: conversations, isLoading: conversationsLoading } = useConversations();
  const createConversation = useCreateConversation();
  const deleteConversation = useDeleteConversation();

  const renameMutation = useMutation({
    mutationFn: ({ id, title }: { id: string; title: string }) =>
      apiFetch<Conversation>(`/api/conversations/${id}`, { method: "PATCH", json: { title } }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["conversations"] }),
    onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to rename conversation"),
  });

  const { data: detail, isFetching: detailLoading } = useQuery({
    queryKey: ["conversation", selectedId],
    queryFn: () => {
      if (selectedId === null) throw new Error("no conversation selected");
      return apiFetch<ConversationDetail>(`/api/conversations/${selectedId}`);
    },
    enabled: selectedId !== null,
  });

  // Seed the thread once per selected conversation. The create-then-send path
  // presets seededRef so the (empty) history fetch cannot clobber the
  // optimistic messages the run just appended.
  useEffect(() => {
    if (!detail || !selectedId || detail.conversation.id !== selectedId) return;
    if (seededRef.current === selectedId) return;
    seededRef.current = selectedId;
    setMessages(detail.messages.map(toChatMessage));
  }, [detail, selectedId, setMessages]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [messages]);

  // Don't keep a stream (or its setState calls) alive after leaving the page.
  useEffect(() => () => stop(), [stop]);

  function selectConversation(id: string) {
    if (id === selectedId) return;
    reset();
    seededRef.current = null;
    queryClient.removeQueries({ queryKey: ["conversation", id] });
    setSendError(null);
    setSelectedId(id);
  }

  function startNewChat() {
    reset();
    seededRef.current = null;
    setSendError(null);
    setSelectedId(null);
  }

  function renameConversation(c: Conversation) {
    const next = window.prompt("Rename conversation", c.title);
    if (next === null) return;
    const title = next.trim();
    if (!title || title === c.title) return;
    renameMutation.mutate({ id: c.id, title });
  }

  function confirmDelete() {
    if (!pendingDelete) return;
    const id = pendingDelete.id;
    deleteConversation.mutate(id, {
      onSuccess: () => {
        if (id === selectedId) {
          reset();
          setSelectedId(null);
        }
        setPendingDelete(null);
      },
      onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to delete conversation"),
    });
  }

  async function submit() {
    const content = input.trim();
    if (!content || status === "running" || createConversation.isPending) return;
    setInput("");
    setSendError(null);
    try {
      let conversationId = selectedId;
      if (conversationId === null) {
        const conversation = await createConversation.mutateAsync();
        conversationId = conversation.id;
        seededRef.current = conversationId;
        setSelectedId(conversationId);
      }
      await send(content, conversationId);
    } catch (err) {
      setSendError(err instanceof ApiError ? err.message : "Something went wrong. Please try again.");
    }
  }

  async function logout() {
    try {
      await apiFetch("/api/auth/logout", { method: "POST" });
    } catch { /* clearing local state regardless */ }
    useAuth.getState().clear();
    navigate("/login", { replace: true });
  }

  const activeConversation = conversations?.find((c) => c.id === selectedId);
  const running = status === "running";

  return (
    <div className="flex h-svh bg-background">
      <aside className="flex w-72 shrink-0 flex-col border-r">
        <div className="flex items-center justify-between gap-2 p-3">
          <span className="font-semibold text-sm">Golem</span>
          <Button variant="outline" size="sm" onClick={startNewChat}>
            <Plus /> New chat
          </Button>
        </div>
        <ScrollArea className="min-h-0 flex-1">
          <nav className="space-y-0.5 px-2 pb-3">
            {conversationsLoading && (
              <div className="space-y-2 px-1 pt-1">
                {[0, 1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-9 w-full" />
                ))}
              </div>
            )}
            {conversations?.length === 0 && (
              <p className="px-3 py-6 text-muted-foreground text-sm">No conversations yet.</p>
            )}
            {conversations?.map((c) => (
              <div
                key={c.id}
                className={cn(
                  "group flex items-center gap-1 rounded-md pr-1",
                  c.id === selectedId ? "bg-accent text-accent-foreground" : "hover:bg-accent/50",
                )}
              >
                <button
                  type="button"
                  onClick={() => selectConversation(c.id)}
                  title={c.title || "Untitled"}
                  className="min-w-0 flex-1 truncate px-3 py-2 text-left text-sm outline-none"
                >
                  {c.title || "Untitled"}
                </button>
                <div className="hidden shrink-0 gap-0.5 group-focus-within:flex group-hover:flex">
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    aria-label={`Rename ${c.title || "conversation"}`}
                    onClick={() => renameConversation(c)}
                  >
                    <Pencil />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    aria-label={`Delete ${c.title || "conversation"}`}
                    className="text-destructive hover:text-destructive"
                    onClick={() => setPendingDelete(c)}
                  >
                    <Trash2 />
                  </Button>
                </div>
              </div>
            ))}
          </nav>
        </ScrollArea>
      </aside>

      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between gap-2 border-b px-4 py-2.5">
          <h1 className="min-w-0 truncate text-sm font-medium">
            {activeConversation?.title || (selectedId ? "Conversation" : "New chat")}
          </h1>
          <div className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="sm" asChild>
              <Link to="/usage">Usage</Link>
            </Button>
            <Button variant="ghost" size="icon-sm" aria-label="Log out" onClick={() => void logout()}>
              <LogOut />
            </Button>
          </div>
        </header>

        <ScrollArea className="min-h-0 flex-1">
          <div className="mx-auto w-full max-w-3xl space-y-4 px-4 py-6">
            {messages.length === 0 && !detailLoading && (
              <div className="flex h-[60vh] items-center justify-center px-4 text-center text-muted-foreground text-sm">
                {selectedId
                  ? "No messages yet — say hello."
                  : "Pick a conversation on the left, or send a message to start a new chat."}
              </div>
            )}
            {messages.map((m) => (
              <div key={m.id} className={cn("flex", m.role === "user" ? "justify-end" : "justify-start")}>
                {m.role === "user" ? (
                  <div className="max-w-[75%] rounded-2xl rounded-br-sm bg-primary px-4 py-2 whitespace-pre-wrap text-primary-foreground text-sm">
                    {m.content}
                  </div>
                ) : (
                  <div className="w-full max-w-[85%] space-y-1">
                    {m.streaming && trace.length > 0 && <ThinkingTrace rows={trace} />}
                    {m.streaming && m.content === "" ? (
                      <RunLoader />
                    ) : (
                      <StreamingText content={m.content} streaming={m.streaming} />
                    )}
                    {m.error && <p className="text-destructive text-sm">{m.error}</p>}
                    {m.truncated && <Badge variant="outline" className="text-muted-foreground text-xs">stopped early</Badge>}
                  </div>
                )}
              </div>
            ))}
            <div ref={bottomRef} />
          </div>
        </ScrollArea>

        <div className="border-t p-3">
          <div className="mx-auto w-full max-w-3xl">
            {sendError && <p className="mb-2 text-destructive text-sm">{sendError}</p>}
            <div className="flex items-end gap-2">
              <textarea
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && !e.shiftKey) {
                    e.preventDefault();
                    void submit();
                  }
                }}
                rows={1}
                placeholder="Message Golem…"
                disabled={running}
                className="max-h-48 min-h-9 w-full resize-none rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none field-sizing-content placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:opacity-50"
              />
              {running ? (
                <Button size="icon" variant="outline" onClick={stop} aria-label="Stop generating">
                  <Square />
                </Button>
              ) : (
                <Button
                  size="icon"
                  onClick={() => void submit()}
                  disabled={!input.trim() || createConversation.isPending}
                  aria-label="Send message"
                >
                  <Send />
                </Button>
              )}
            </div>
          </div>
        </div>
      </main>

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
    </div>
  );
}
