import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  BookOpen,
  Check,
  Copy,
  Flame,
  Gauge,
  Globe,
  LogOut,
  Menu,
  Mic,
  Pencil,
  PenLine,
  Plus,
  Paperclip,
  RefreshCw,
  Search,
  Send,
  ShieldCheck,
  Sparkles,
  Square,
  Trash2,
  Wrench,
  X,
} from "lucide-react";
import { ApiError, apiFetch } from "@/lib/api";
import { MAX_IMAGES_PER_MESSAGE, readImageFiles, type PendingImage } from "@/lib/images";
import { cn, fmtTokens } from "@/lib/utils";
import type { ChatMessage, Conversation, ConversationSettings } from "@/lib/types";
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
import { RunLoader } from "@/components/ai/RunLoader";
import { StreamingText } from "@/components/ai/StreamingText";
import { ThinkingTrace } from "@/components/ai/ThinkingTrace";
import { ConversationSettingsDialog } from "@/features/chat/ConversationSettingsDialog";
import { useChat } from "@/features/chat/useChat";
import { useModels } from "@/features/chat/useModels";
import { useVoiceInput } from "@/features/chat/useVoiceInput";
import {
  useConversations,
  useCreateConversation,
  useDeleteConversation,
  useUpdateConversationSettings,
} from "@/features/chat/useConversations";
import { useTools } from "@/features/tools/useTools";

interface ConversationDetail {
  conversation: Conversation;
  messages: ChatMessage[];
}

// Server messages arrive without the ephemeral streaming/error fields.
function toChatMessage(m: ChatMessage): ChatMessage {
  return { id: m.id, role: m.role, content: m.content, truncated: m.truncated, createdAt: m.createdAt, usage: m.usage, images: m.images };
}

// CopyMessageButton is a small hover action that copies one message's text.
function CopyMessageButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      aria-label="Copy message"
      title="Copy"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text);
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        } catch { /* clipboard unavailable */ }
      }}
      className="rounded-md p-1 text-muted-foreground/60 transition-colors hover:text-foreground"
    >
      {copied ? <Check className="size-3.5 text-emerald-600" /> : <Copy className="size-3.5" />}
    </button>
  );
}

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

const SUGGESTIONS = [
  { icon: Globe, label: "What's the weather in Tokyo?" },
  { icon: Flame, label: "Search Hacker News for agent frameworks" },
  { icon: BookOpen, label: "Summarize the Wikipedia article on golems" },
  { icon: PenLine, label: "Draft a short launch post for a CLI tool" },
];

export function ChatPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const user = useAuth((s) => s.user);
  // The selected conversation lives in the URL (/chat/:conversationId?), so
  // refresh and deep links land on the same thread.
  const { conversationId } = useParams();
  const selectedId = conversationId ?? null;
  const [input, setInput] = useState("");
  const [sendError, setSendError] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Conversation | null>(null);
  const [renaming, setRenaming] = useState<Conversation | null>(null);
  const [renameTitle, setRenameTitle] = useState("");
  const [filter, setFilter] = useState("");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  // The user message currently being edited inline (null = none).
  const [editing, setEditing] = useState<{ id: string; draft: string } | null>(null);
  // Images picked for the next message (cleared on send).
  const [attachments, setAttachments] = useState<PendingImage[]>([]);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  // Settings for a chat that does not exist yet; applied at create time.
  const [draftSettings, setDraftSettings] = useState<ConversationSettings>({
    model: "",
    temperature: null,
    systemPrompt: "",
  });
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const seededRef = useRef<string | null>(null);

  const { messages, setMessages, status, send, edit, regenerate, resolve, stop, reset, trace, pending } = useChat(() => {
    // Refresh titles/order after each completed run.
    void queryClient.invalidateQueries({ queryKey: ["conversations"] });
  });

  const { data: conversations, isLoading: conversationsLoading } = useConversations();
  const { data: tools } = useTools();
  const { data: models } = useModels();
  const createConversation = useCreateConversation();
  const deleteConversation = useDeleteConversation();
  const updateSettings = useUpdateConversationSettings(selectedId);

  const renameMutation = useMutation({
    mutationFn: ({ id, title }: { id: string; title: string }) =>
      apiFetch<Conversation>(`/api/conversations/${id}`, { method: "PATCH", json: { title } }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["conversations"] }),
    onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to rename conversation"),
  });

  const { data: detail, isFetching: detailLoading, isError: detailError } = useQuery({
    queryKey: ["conversation", selectedId],
    queryFn: () => {
      if (selectedId === null) throw new Error("no conversation selected");
      return apiFetch<ConversationDetail>(`/api/conversations/${selectedId}`);
    },
    enabled: selectedId !== null,
    retry: false,
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

  // A URL id that doesn't exist (deep link, deleted elsewhere) falls back to
  // a fresh chat.
  useEffect(() => {
    if (detailError) navigate("/chat", { replace: true });
  }, [detailError, navigate]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [messages]);

  // Don't keep a stream (or its setState calls) alive after leaving the page.
  useEffect(() => () => stop(), [stop]);

  function selectConversation(id: string) {
    if (id === selectedId) {
      setSidebarOpen(false);
      return;
    }
    reset();
    seededRef.current = null;
    queryClient.removeQueries({ queryKey: ["conversation", id] });
    setSendError(null);
    navigate(`/chat/${id}`);
    setSidebarOpen(false);
  }

  function startNewChat() {
    reset();
    seededRef.current = null;
    setSendError(null);
    navigate("/chat");
    setSidebarOpen(false);
  }

  function openRename(c: Conversation) {
    setRenaming(c);
    setRenameTitle(c.title);
  }

  function confirmRename() {
    if (!renaming) return;
    const title = renameTitle.trim();
    if (!title || title === renaming.title) {
      setRenaming(null);
      return;
    }
    renameMutation.mutate(
      { id: renaming.id, title },
      { onSuccess: () => setRenaming(null) },
    );
  }

  function confirmDelete() {
    if (!pendingDelete) return;
    const id = pendingDelete.id;
    deleteConversation.mutate(id, {
      onSuccess: () => {
        if (id === selectedId) {
          reset();
          navigate("/chat");
        }
        setPendingDelete(null);
      },
      onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to delete conversation"),
    });
  }

  async function submit(preset?: string) {
    const content = (preset ?? input).trim();
    if ((!content && attachments.length === 0) || status === "running" || createConversation.isPending) return;
    if (selectedId && pending.length > 0) {
      toast.error("Resolve the pending approval first.");
      return;
    }
    if (preset === undefined) setInput("");
    setSendError(null);
    const images = attachments.map(({ mediaType, data }) => ({ mediaType, data }));
    setAttachments([]);
    try {
      let conversationId = selectedId;
      if (conversationId === null) {
        const conversation = await createConversation.mutateAsync(draftSettings);
        conversationId = conversation.id;
        seededRef.current = conversationId;
        navigate(`/chat/${conversationId}`, { replace: true });
      }
      await send(content, conversationId, images);
    } catch (err) {
      setSendError(err instanceof ApiError ? err.message : "Something went wrong. Please try again.");
    }
  }

  async function pickImages(files: FileList | null) {
    if (!files || files.length === 0) return;
    const { images, errors } = await readImageFiles([...files]);
    for (const message of errors) toast.error(message);
    setAttachments((prev) => [...prev, ...images].slice(0, MAX_IMAGES_PER_MESSAGE));
    if (fileInputRef.current) fileInputRef.current.value = "";
  }

  function removeAttachment(previewUrl: string) {
    setAttachments((prev) => prev.filter((a) => a.previewUrl !== previewUrl));
  }

  function decide(approved: boolean) {
    if (!selectedId || pending.length === 0) return;
    const decisions = pending.map((p) => ({ callId: p.callId, approved, reason: approved ? undefined : "user denied" }));
    void resolve(selectedId, decisions).catch((err) => {
      setSendError(err instanceof ApiError ? err.message : "Failed to resume the conversation.");
    });
  }

  function saveEdit(msgId: string) {
    if (!editing || selectedId === null) return;
    const content = editing.draft.trim();
    if (!content) return;
    setEditing(null);
    setSendError(null);
    void edit(selectedId, msgId, content).catch((err) => {
      setSendError(err instanceof ApiError ? err.message : "Failed to re-run the conversation.");
    });
  }

  function regenerateLast() {
    if (selectedId === null) return;
    setSendError(null);
    void regenerate(selectedId).catch((err) => {
      setSendError(err instanceof ApiError ? err.message : "Failed to regenerate the answer.");
    });
  }

  // Dictation keeps whatever is already drafted and appends transcripts to
  // it; interim fragments compose on top of the finalized ones.
  const voiceBaseRef = useRef("");
  const voiceTranscriptRef = useRef("");
  const voice = useVoiceInput({
    onInterim: (t) => {
      const composed = [voiceBaseRef.current, `${voiceTranscriptRef.current} ${t}`.trim()].filter(Boolean).join(" ");
      setInput(composed);
    },
    onFinal: (t) => {
      voiceTranscriptRef.current = `${voiceTranscriptRef.current} ${t}`.trim();
      setInput([voiceBaseRef.current, voiceTranscriptRef.current].filter(Boolean).join(" "));
    },
  });

  function toggleVoice() {
    if (voice.listening) {
      voice.toggle();
      return;
    }
    voiceBaseRef.current = input;
    voiceTranscriptRef.current = "";
    voice.toggle();
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
  const showFilters = (conversations?.length ?? 0) >= 6;

  // The settings the composer's model chip shows and the dialog edits: the
  // stored conversation's when one is open, the draft otherwise.
  const activeSettings: ConversationSettings = activeConversation
    ? {
        model: activeConversation.model || models?.default || "",
        temperature: activeConversation.temperature,
        systemPrompt: activeConversation.systemPrompt,
      }
    : draftSettings;

  function modelLabel(modelID: string): string {
    if (!modelID) return models?.models.find((m) => m.id === models.default)?.label ?? "Model";
    return models?.models.find((m) => m.id === modelID)?.label ?? modelID;
  }

  function applySettings(settings: ConversationSettings) {
    if (selectedId === null) {
      setDraftSettings(settings);
      setSettingsOpen(false);
      return;
    }
    updateSettings.mutate(settings, {
      onSuccess: () => setSettingsOpen(false),
      onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to save settings"),
    });
  }

  const visibleConversations = (conversations ?? []).filter((c) =>
    (c.title || "Untitled").toLowerCase().includes(filter.trim().toLowerCase()),
  );
  const groups = GROUP_ORDER.map((label) => ({
    label,
    items: visibleConversations.filter((c) => groupKey(c.updatedAt) === label),
  })).filter((g) => g.items.length > 0);
  const nothingMatches =
    !conversationsLoading && visibleConversations.length === 0 && (conversations?.length ?? 0) > 0;

  return (
    <div className="flex h-svh bg-background">
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/40 md:hidden"
          aria-hidden="true"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      <aside
        className={cn(
          "flex w-72 shrink-0 flex-col border-r bg-background",
          "max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:z-40 max-md:shadow-xl",
          "max-md:transition-transform max-md:duration-200 max-md:-translate-x-full",
          sidebarOpen && "max-md:translate-x-0",
        )}
      >
        <div className="flex items-center gap-2 p-3">
          <BrandMark className="size-7" />
          <span className="font-semibold text-sm tracking-tight">Golem</span>
          <Button
            variant="ghost"
            size="icon-sm"
            className="ml-auto md:hidden"
            aria-label="Close conversations"
            onClick={() => setSidebarOpen(false)}
          >
            <X />
          </Button>
        </div>

        <div className="px-3 pb-2">
          <Button size="sm" className="w-full" onClick={startNewChat}>
            <Plus /> New chat
          </Button>
        </div>

        {showFilters && (
          <div className="px-3 pb-2">
            <div className="relative">
              <Search className="absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <input
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                placeholder="Search conversations"
                aria-label="Search conversations"
                className="h-8 w-full rounded-md border border-input bg-transparent pr-2 pl-7 text-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
              />
            </div>
          </div>
        )}

        <ScrollArea className="min-h-0 flex-1">
          <nav className="space-y-0.5 px-2 pb-3">
            {conversationsLoading && (
              <div className="space-y-2 px-1 pt-1">
                {[0, 1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-9 w-full" />
                ))}
              </div>
            )}
            {!conversationsLoading && (conversations?.length ?? 0) === 0 && (
              <p className="px-3 py-6 text-muted-foreground text-sm">
                No conversations yet. Send a message to start one.
              </p>
            )}
            {nothingMatches && (
              <p className="px-3 py-6 text-muted-foreground text-sm">No conversations match.</p>
            )}
            {groups.map((group) => (
              <div key={group.label}>
                <p className="px-3 pt-3 pb-1 text-[11px] font-medium tracking-wider text-muted-foreground/60 uppercase">
                  {group.label}
                </p>
                {group.items.map((c) => (
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
                        onClick={() => openRename(c)}
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
              </div>
            ))}
          </nav>
        </ScrollArea>

        <div className="border-t p-3">
          <div className="flex items-center gap-2.5">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold uppercase">
              {(user?.email ?? "?").slice(0, 1)}
            </span>
            <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground" title={user?.email}>
              {user?.email ?? "Signed in"}
            </span>
            <Button variant="ghost" size="icon-sm" aria-label="Log out" onClick={() => void logout()}>
              <LogOut />
            </Button>
          </div>
        </div>
      </aside>

      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center gap-1 border-b px-4 py-2.5">
          <Button
            variant="ghost"
            size="icon-sm"
            className="md:hidden"
            aria-label="Open conversations"
            onClick={() => setSidebarOpen(true)}
          >
            <Menu />
          </Button>
          <h1 className="min-w-0 flex-1 truncate text-sm font-medium">
            {activeConversation?.title || (selectedId ? "Conversation" : "New chat")}
          </h1>
          <Button variant="ghost" size="sm" asChild>
            <Link to="/tools">
              <Wrench /> Tools
            </Link>
          </Button>
          <Button variant="ghost" size="sm" asChild>
            <Link to="/usage">
              <Gauge /> Usage
            </Link>
          </Button>
        </header>

        <ScrollArea className="min-h-0 flex-1">
          <div className="mx-auto w-full max-w-3xl px-4 py-6">
            {messages.length === 0 && !detailLoading ? (
              <div className="flex min-h-[55vh] flex-col items-center justify-center gap-6 text-center">
                <div className="space-y-2.5">
                  <BrandMark className="mx-auto size-12 rounded-xl [&_svg]:size-6" />
                  <h2 className="text-xl font-semibold tracking-tight">How can I help?</h2>
                  <p className="max-w-sm text-balance text-muted-foreground text-sm">
                    Ask anything{tools && tools.length > 0 ? ` — ${tools.length} ${tools.length === 1 ? "tool is" : "tools are"} ready to call` : ""}.
                  </p>
                </div>
                <div className="grid w-full max-w-lg gap-2 sm:grid-cols-2">
                  {SUGGESTIONS.map((s, i) => (
                    <button
                      key={s.label}
                      type="button"
                      onClick={() => void submit(s.label)}
                      style={{ animationDelay: `${i * 50}ms` }}
                      className="group flex items-center gap-2.5 rounded-xl border bg-card p-3 text-left text-sm shadow-xs transition-all duration-200
                        hover:-translate-y-0.5 hover:border-foreground/25 hover:shadow-md hover:shadow-black/5
                        active:translate-y-0 active:scale-[0.99]
                        focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50
                        animate-in fade-in slide-in-from-bottom-2 duration-300 [animation-fill-mode:backwards] motion-reduce:animate-none"
                    >
                      <span className="flex size-7 shrink-0 items-center justify-center rounded-lg border bg-muted/60 text-foreground/80">
                        <s.icon className="size-3.5" />
                      </span>
                      <span className="min-w-0 text-muted-foreground group-hover:text-foreground">{s.label}</span>
                    </button>
                  ))}
                </div>
              </div>
            ) : (
              <div className="space-y-5">
                {messages.map((m, i) => {
                  const isLast = i === messages.length - 1;
                  const blockInteraction = running || pending.length > 0;
                  return (
                    <div key={m.id} className={cn("flex", m.role === "user" ? "justify-end" : "group gap-3")}>
                      {m.role === "user" ? (
                        editing?.id === m.id ? (
                          <div className="w-full max-w-[85%] rounded-2xl border border-ring/60 bg-card p-2 shadow-sm">
                            <textarea
                              value={editing.draft}
                              onChange={(e) => setEditing({ id: m.id, draft: e.target.value })}
                              onKeyDown={(e) => {
                                if (e.key === "Enter" && !e.shiftKey) {
                                  e.preventDefault();
                                  saveEdit(m.id);
                                } else if (e.key === "Escape") {
                                  setEditing(null);
                                }
                              }}
                              rows={2}
                              autoFocus
                              className="max-h-48 w-full resize-none bg-transparent px-1.5 py-1 text-sm outline-none field-sizing-content"
                            />
                            <div className="flex justify-end gap-1.5">
                              <Button size="sm" variant="outline" onClick={() => setEditing(null)}>
                                Cancel
                              </Button>
                              <Button size="sm" disabled={!editing.draft.trim() || blockInteraction} onClick={() => saveEdit(m.id)}>
                                Save & resend
                              </Button>
                            </div>
                          </div>
                        ) : (
                          <div className="group flex max-w-[75%] flex-col items-end gap-1">
                            {m.images && m.images.length > 0 && (
                              <div className="flex flex-wrap justify-end gap-1.5">
                                {m.images.map((img, idx) => (
                                  <img
                                    key={idx}
                                    src={img.dataUrl}
                                    alt="attachment"
                                    className="max-h-48 max-w-[16rem] rounded-xl border object-cover shadow-sm"
                                  />
                                ))}
                              </div>
                            )}
                            {m.content && (
                              <div className="rounded-2xl rounded-br-md bg-primary px-4 py-2.5 whitespace-pre-wrap text-primary-foreground text-sm shadow-sm">
                                {m.content}
                              </div>
                            )}
                            <button
                              type="button"
                              aria-label="Edit message"
                              title="Edit & resend"
                              disabled={blockInteraction}
                              onClick={() => setEditing({ id: m.id, draft: m.content })}
                              className="rounded-md p-1 text-muted-foreground/60 opacity-0 transition-opacity group-hover:opacity-100 hover:text-foreground disabled:cursor-not-allowed"
                            >
                              <Pencil className="size-3.5" />
                            </button>
                          </div>
                        )
                      ) : (
                        <>
                          <BrandMark className="mt-0.5 size-7" />
                          <div className="min-w-0 flex-1 space-y-1 pt-0.5">
                            {m.streaming && trace.length > 0 && <ThinkingTrace rows={trace} />}
                            {m.streaming && m.content === "" ? (
                              <RunLoader />
                            ) : (
                              <StreamingText content={m.content} streaming={m.streaming} />
                            )}
                            {m.error && <p className="text-destructive text-sm">{m.error}</p>}
                            {m.truncated && (
                              <Badge variant="outline" className="text-muted-foreground text-xs">stopped early</Badge>
                            )}
                            {!m.streaming && (
                              <div className="flex items-center gap-1.5 pt-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
                                <CopyMessageButton text={m.content} />
                                {isLast && (
                                  <button
                                    type="button"
                                    aria-label="Regenerate answer"
                                    title="Regenerate"
                                    disabled={blockInteraction}
                                    onClick={regenerateLast}
                                    className="rounded-md p-1 text-muted-foreground/60 transition-colors hover:text-foreground disabled:cursor-not-allowed"
                                  >
                                    <RefreshCw className="size-3.5" />
                                  </button>
                                )}
                                {m.usage && (
                                  <p className="text-[11px] text-muted-foreground/60">
                                    {fmtTokens(m.usage.inputTokens)} in / {fmtTokens(m.usage.outputTokens)} out · $
                                    {m.usage.costUsd.toFixed(5)}
                                    {m.usage.model ? ` · ${m.usage.model}` : ""}
                                  </p>
                                )}
                              </div>
                            )}
                          </div>
                        </>
                      )}
                    </div>
                  );
                })}
                {pending.length > 0 && (
                  <div className="overflow-hidden rounded-xl border border-amber-500/40 bg-amber-500/5">
                    <div className="flex items-center gap-2 border-b border-amber-500/20 px-4 py-3">
                      <ShieldCheck className="size-4 shrink-0 text-amber-600 dark:text-amber-400" />
                      <p className="text-sm font-medium">
                        The agent wants to run {pending.length === 1 ? "a tool" : `${pending.length} tools`}
                      </p>
                    </div>
                    <div className="space-y-2 p-3">
                      {pending.map((p) => (
                        <div key={p.callId} className="rounded-lg border bg-background/70 px-3 py-2.5">
                          <div className="flex items-center gap-2">
                            <Badge variant="outline" className="font-mono text-xs">{p.toolName}</Badge>
                            <span className="truncate font-mono text-xs text-muted-foreground">
                              {JSON.stringify(p.args)}
                            </span>
                          </div>
                          {p.reason && <p className="mt-1 text-xs text-muted-foreground">{p.reason}</p>}
                        </div>
                      ))}
                    </div>
                    <div className="flex gap-2 px-3 pb-3">
                      <Button size="sm" onClick={() => decide(true)} disabled={running}>
                        Approve
                      </Button>
                      <Button size="sm" variant="outline" onClick={() => decide(false)} disabled={running}>
                        Deny
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            )}
            <div ref={bottomRef} />
          </div>
        </ScrollArea>

        <div className="border-t p-3">
          <div className="mx-auto w-full max-w-3xl">
            {sendError && <p className="mb-2 text-destructive text-sm">{sendError}</p>}
            {attachments.length > 0 && (
              <div className="mb-2 flex flex-wrap gap-2">
                {attachments.map((a) => (
                  <div key={a.previewUrl} className="group/img relative">
                    <img src={a.previewUrl} alt={a.name} className="size-16 rounded-lg border object-cover" />
                    <button
                      type="button"
                      aria-label={`Remove ${a.name}`}
                      onClick={() => removeAttachment(a.previewUrl)}
                      className="absolute -top-1.5 -right-1.5 rounded-full border bg-background p-0.5 text-muted-foreground shadow-sm hover:text-foreground"
                    >
                      <X className="size-3" />
                    </button>
                  </div>
                ))}
              </div>
            )}
            <div
              className={cn(
                "flex items-end gap-1.5 rounded-2xl border bg-card p-1.5 shadow-sm transition-all",
                "focus-within:border-ring/60 focus-within:ring-[3px] focus-within:ring-ring/20",
                pending.length > 0 && "opacity-60",
              )}
            >
              <Button
                variant="ghost"
                size="sm"
                className="mb-0.5 shrink-0 gap-1.5 rounded-xl text-muted-foreground text-xs hover:text-foreground"
                onClick={() => setSettingsOpen(true)}
                title="Conversation settings"
              >
                <Sparkles />
                <span className="hidden sm:inline">{modelLabel(activeSettings.model)}</span>
              </Button>
              <input
                ref={fileInputRef}
                type="file"
                accept="image/png,image/jpeg,image/webp,image/gif"
                multiple
                hidden
                onChange={(e) => void pickImages(e.target.files)}
              />
              <Button
                variant="ghost"
                size="icon"
                className="mb-0.5 shrink-0 rounded-xl text-muted-foreground"
                onClick={() => fileInputRef.current?.click()}
                disabled={running || pending.length > 0 || attachments.length >= MAX_IMAGES_PER_MESSAGE}
                aria-label="Attach images"
                title="Attach images"
              >
                <Paperclip />
              </Button>
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
                placeholder={pending.length > 0 ? "Waiting for approval…" : "Message Golem…"}
                disabled={running || pending.length > 0}
                className="max-h-48 min-h-9 flex-1 resize-none bg-transparent px-2.5 py-1.5 text-sm outline-none field-sizing-content placeholder:text-muted-foreground disabled:cursor-not-allowed"
              />
              {voice.supported && (
                <Button
                  size="icon"
                  variant={voice.listening ? "default" : "ghost"}
                  className="shrink-0 rounded-xl text-muted-foreground"
                  onClick={toggleVoice}
                  disabled={running || pending.length > 0}
                  aria-label={voice.listening ? "Stop dictation" : "Start dictation"}
                  title={voice.listening ? "Stop dictation" : "Dictate"}
                >
                  {voice.listening ? <Square /> : <Mic />}
                </Button>
              )}
              {running ? (
                <Button
                  size="icon"
                  variant="outline"
                  className="shrink-0 rounded-xl"
                  onClick={stop}
                  aria-label="Stop generating"
                >
                  <Square />
                </Button>
              ) : (
                <Button
                  size="icon"
                  className="shrink-0 rounded-xl"
                  onClick={() => void submit()}
                  disabled={(!input.trim() && attachments.length === 0) || createConversation.isPending}
                  aria-label="Send message"
                >
                  <Send />
                </Button>
              )}
            </div>
            <p className="px-1 pt-1.5 text-[11px] text-muted-foreground/60">
              {pending.length > 0
                ? "Resolve the approval above to continue."
                : "Enter to send · Shift+Enter for a new line"}
            </p>
          </div>
        </div>
      </main>

      <ConversationSettingsDialog
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        models={models}
        settings={activeSettings}
        onApply={applySettings}
        saving={updateSettings.isPending}
      />

      <Dialog open={renaming !== null} onOpenChange={(open) => { if (!open) setRenaming(null); }}>
        <DialogContent showCloseButton={false} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Rename conversation</DialogTitle>
            <DialogDescription>Give this conversation a title you'll recognize.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="rename-title">Title</Label>
            <Input
              id="rename-title"
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
