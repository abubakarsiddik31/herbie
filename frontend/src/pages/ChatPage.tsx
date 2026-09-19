import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowUp,
  BookOpen,
  Bot,
  Calendar,
  Check,
  ChevronDown,
  Copy,
  Download,
  Eye,
  FileCode,
  Globe,
  HelpCircle,
  Loader2,
  Mic,
  Paperclip,
  Pencil,
  PenLine,
  RefreshCw,
  Share2,
  ShieldCheck,
  Sidebar,
  SlidersHorizontal,
  Square,
  Trash2,
  X,
} from "lucide-react";
import { ApiError, apiFetch } from "@/lib/api";
import { ALLOWED_IMAGE_TYPES, MAX_IMAGES_PER_MESSAGE, readImageFiles, type PendingImage } from "@/lib/images";
import {
  type AttachedFile,
  MAX_ATTACHED_FILES,
  MAX_ATTACHED_CHARS,
  isSupportedDocOrCodeFile,
  processAttachedFile,
  formatPromptWithFiles,
  extractFilesAndPrompt,
} from "@/lib/files";
import { cn, fmtTokens, cleanConversationTitle } from "@/lib/utils";
import type { ChatMessage, Conversation, ConversationSettings } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ParsedFileViewerDialog } from "@/features/documents/ParsedFileViewerDialog";
import { useChatApps, extractAppConnectProviders, type ChatApp } from "@/features/chat/useChatApps";
import { MentionMenu, MentionAppIcon } from "@/features/chat/MentionMenu";
import { AppConnectCard } from "@/features/chat/AppConnectCard";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import { HarveyAvatar } from "@/components/HarveyAvatar";
import { RunLoader } from "@/components/ai/RunLoader";
import { StreamingText } from "@/components/ai/StreamingText";
import { ThinkingTrace } from "@/components/ai/ThinkingTrace";
import { useSidebar } from "@/components/layout/SidebarContext";
import { CommandPalette } from "@/features/chat/CommandPalette";
import { ConversationSettingsDialog } from "@/features/chat/ConversationSettingsDialog";
import { conversationFilename, downloadMarkdown, toMarkdown } from "@/features/chat/exportMarkdown";
import { ShareDialog } from "@/features/chat/ShareDialog";
import { ShortcutsDialog } from "@/features/chat/ShortcutsDialog";
import { useChat } from "@/features/chat/useChat";
import { useModels } from "@/features/chat/useModels";
import { useVoiceInput } from "@/features/chat/useVoiceInput";
import {
  useConversations,
  useCreateConversation,
  useDeleteMessage,
  useUpdateConversationSettings,
} from "@/features/chat/useConversations";
import { useTools } from "@/features/tools/useTools";
import { useDocuments } from "@/features/documents/useDocuments";

interface ConversationDetail {
  conversation: Conversation;
  messages: ChatMessage[];
}

function toChatMessage(m: ChatMessage): ChatMessage {
  return {
    id: m.id,
    role: m.role,
    content: m.content,
    truncated: m.truncated,
    createdAt: m.createdAt,
    usage: m.usage,
    images: m.images,
    sources: m.sources,
  };
}

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

const SUGGESTIONS = [
  { icon: Calendar, label: "@calendar What meetings do I have scheduled today?" },
  { icon: Globe, label: "@web What's the latest news in AI agents?" },
  { icon: BookOpen, label: "Explain quantum computing in simple terms" },
  { icon: PenLine, label: "Draft a short launch post for an open source tool" },
];

export function ChatPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const { conversationId } = useParams();
  const selectedId = conversationId ?? null;

  const { toggleSidebar, setMobileOpen } = useSidebar();

  const [input, setInput] = useState("");
  const [sendError, setSendError] = useState<string | null>(null);
  const [pendingMessageDelete, setPendingMessageDelete] = useState<string | null>(null);
  const [sharing, setSharing] = useState<Conversation | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const [editing, setEditing] = useState<{ id: string; draft: string } | null>(null);
  const [attachments, setAttachments] = useState<PendingImage[]>([]);
  const [attachedFiles, setAttachedFiles] = useState<AttachedFile[]>([]);
  const [extractingFiles, setExtractingFiles] = useState(false);
  const [viewingFile, setViewingFile] = useState<{
    title: string;
    documentId?: string | null;
    content?: string | null;
    sizeBytes?: number;
    status?: string;
    chunkCount?: number;
  } | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  // Tool Apps & Mention Autocomplete
  const { filterApps, getAppByMention } = useChatApps();
  const [mentionQuery, setMentionQuery] = useState<string | null>(null);
  const [mentionSelectedIndex, setMentionSelectedIndex] = useState(0);
  const filteredApps = filterApps(mentionQuery ?? "");

  useEffect(() => {
    const connected = searchParams.get("connected");
    const error = searchParams.get("error");
    if (connected) {
      const appName = connected === "google_calendar" ? "Google Calendar" : connected.toUpperCase();
      toast.success(`${appName} connected successfully! You can now use @${connected === "google_calendar" ? "calendar" : connected}.`);
      searchParams.delete("connected");
      setSearchParams(searchParams, { replace: true });
    } else if (error) {
      toast.error(`OAuth connection error: ${error}`);
      searchParams.delete("error");
      setSearchParams(searchParams, { replace: true });
    }

    const mention = searchParams.get("mention");
    if (mention) {
      setInput(`@${mention} `);
      searchParams.delete("mention");
      setSearchParams(searchParams, { replace: true });
      setTimeout(() => {
        if (textareaRef.current) {
          textareaRef.current.focus();
          const len = mention.length + 2;
          textareaRef.current.setSelectionRange(len, len);
        }
      }, 0);
    }
  }, [searchParams, setSearchParams]);

  const detectedApp = useMemo(() => {
    const match = input.match(/@([a-zA-Z0-9_-]+)/);
    if (match) {
      return getAppByMention(match[1]);
    }
    return undefined;
  }, [input, getAppByMention]);

  const handleSelectApp = (app: ChatApp) => {
    if (!textareaRef.current) return;
    const textarea = textareaRef.current;
    const val = textarea.value;
    const cursor = textarea.selectionStart;

    const beforeCursor = val.slice(0, cursor);
    const atIndex = beforeCursor.lastIndexOf("@");
    if (atIndex !== -1) {
      const prefix = val.slice(0, atIndex);
      const suffix = val.slice(cursor);
      const newVal = `${prefix}@${app.mention} ${suffix}`;
      setInput(newVal);
      setMentionQuery(null);
      setTimeout(() => {
        if (textareaRef.current) {
          const newPos = atIndex + app.mention.length + 2;
          textareaRef.current.focus();
          textareaRef.current.setSelectionRange(newPos, newPos);
        }
      }, 0);
    }
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const val = e.target.value;
    setInput(val);
    const cursor = e.target.selectionStart;
    const textBeforeCursor = val.slice(0, cursor);
    const atMatch = textBeforeCursor.match(/@([a-zA-Z0-9_-]*)$/);
    if (atMatch) {
      setMentionQuery(atMatch[1]);
      setMentionSelectedIndex(0);
    } else {
      setMentionQuery(null);
    }
  };

  const handleInputKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (mentionQuery !== null && filteredApps.length > 0) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setMentionSelectedIndex((prev) => (prev + 1) % filteredApps.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setMentionSelectedIndex((prev) => (prev - 1 + filteredApps.length) % filteredApps.length);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        handleSelectApp(filteredApps[mentionSelectedIndex]);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setMentionQuery(null);
        return;
      }
    }

    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void submit();
    }
  };
  const { data: docs } = useDocuments();
  const hasDocs = (docs?.length ?? 0) > 0;
  const [draftSettings, setDraftSettings] = useState<ConversationSettings>({
    model: "",
    temperature: null,
    systemPrompt: "",
    ragEnabled: false,
  });

  useEffect(() => {
    setDraftSettings((prev) => ({ ...prev, ragEnabled: hasDocs }));
  }, [hasDocs]);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const seededRef = useRef<string | null>(null);

  const { messages, setMessages, status, send, edit, regenerate, resolve, stop, reset, trace, pending } = useChat(() => {
    void queryClient.invalidateQueries({ queryKey: ["conversations"] });
  });

  const { data: conversations } = useConversations();
  const { data: tools } = useTools();
  const { data: models } = useModels();
  const createConversation = useCreateConversation();
  const deleteMessage = useDeleteMessage(selectedId);
  const updateSettings = useUpdateConversationSettings(selectedId);

  const { data: detail, isFetching: detailLoading, isError: detailError } = useQuery({
    queryKey: ["conversation", selectedId],
    queryFn: () => {
      if (selectedId === null) throw new Error("no conversation selected");
      return apiFetch<ConversationDetail>(`/api/conversations/${selectedId}`);
    },
    enabled: selectedId !== null,
    retry: false,
  });

  useEffect(() => {
    if (detailError && selectedId !== null) {
      toast.error("Conversation not found");
      navigate("/chat", { replace: true });
    }
  }, [detailError, selectedId, navigate]);

  useEffect(() => {
    if (selectedId === null) {
      seededRef.current = null;
      reset();
      return;
    }
    if (detail && seededRef.current !== selectedId) {
      seededRef.current = selectedId;
      setMessages(detail.messages.map(toChatMessage));
      setSendError(null);
    }
  }, [selectedId, detail, reset, setMessages]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, trace, pending]);

  function startNewChat() {
    navigate("/chat");
    reset();
    setInput("");
    setAttachments([]);
    setAttachedFiles([]);
    setEditing(null);
    setSendError(null);
  }

  async function pickFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    const fileList = Array.from(files);
    const imgFiles: File[] = [];
    const docFiles: File[] = [];

    for (const f of fileList) {
      if ((ALLOWED_IMAGE_TYPES as readonly string[]).includes(f.type)) {
        imgFiles.push(f);
      } else if (isSupportedDocOrCodeFile(f)) {
        docFiles.push(f);
      } else {
        toast.error(`Unsupported file type: ${f.name}`);
      }
    }

    if (imgFiles.length > 0) {
      const { images, errors } = await readImageFiles(imgFiles);
      for (const message of errors) toast.error(message);
      setAttachments((prev) => [...prev, ...images].slice(0, MAX_IMAGES_PER_MESSAGE));
    }

    if (docFiles.length > 0) {
      setExtractingFiles(true);
      try {
        for (const df of docFiles) {
          if (attachedFiles.length >= MAX_ATTACHED_FILES) {
            toast.error(`Maximum ${MAX_ATTACHED_FILES} files per message`);
            break;
          }
          const attached = await processAttachedFile(df);
          const currentChars = attachedFiles.reduce((acc, f) => acc + f.content.length, 0);
          if (currentChars + attached.content.length > MAX_ATTACHED_CHARS) {
            toast.error("Total attached files exceed 100K character limit. Upload to Projects or Documents for retrieval.");
            break;
          }
          setAttachedFiles((prev) => [...prev, attached]);
          toast.success(`Attached ${df.name}`);
        }
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to attach file");
      } finally {
        setExtractingFiles(false);
      }
    }

    if (fileInputRef.current) fileInputRef.current.value = "";
  }

  function removeAttachment(url: string) {
    setAttachments((prev) => prev.filter((a) => a.previewUrl !== url));
  }

  function removeAttachedFile(id: string) {
    setAttachedFiles((prev) => prev.filter((a) => a.id !== id));
  }

  async function submit(overrideText?: string) {
    const text = (overrideText ?? input).trim();
    const totalFiles = attachments.length + attachedFiles.length;
    if ((!text && totalFiles === 0) || running || pending.length > 0 || extractingFiles) return;
    setSendError(null);
    const images = attachments.map(({ mediaType, data }) => ({ mediaType, data }));
    const outgoingText = formatPromptWithFiles(text, attachedFiles);
    if (!overrideText) {
      setInput("");
      setAttachments([]);
      setAttachedFiles([]);
    }

    try {
      let conversationId = selectedId;
      if (conversationId === null) {
        const conv = await createConversation.mutateAsync(draftSettings);
        conversationId = conv.id;
        seededRef.current = conversationId;
        navigate(`/chat/${conversationId}`, { replace: true });
      }
      await send(outgoingText, conversationId, images);
    } catch (err) {
      setSendError(err instanceof ApiError ? err.message : "Failed to send message");
    }
  }

  function regenerateLast() {
    if (selectedId === null || running || pending.length > 0) return;
    setSendError(null);
    regenerate(selectedId).catch((err) =>
      setSendError(err instanceof ApiError ? err.message : "Failed to regenerate answer"),
    );
  }

  function saveEdit(msgId: string) {
    if (selectedId === null || !editing || running || pending.length > 0) return;
    const next = editing.draft.trim();
    if (!next) return;
    setEditing(null);
    setSendError(null);
    edit(selectedId, msgId, next).catch((err) =>
      setSendError(err instanceof ApiError ? err.message : "Failed to resend message"),
    );
  }

  function confirmDeleteMessage() {
    if (!pendingMessageDelete || !selectedId) return;
    const id = pendingMessageDelete;
    deleteMessage.mutate(id, {
      onSuccess: () => {
        setMessages((prev) => {
          const idx = prev.findIndex((m) => m.id === id);
          if (idx === -1) return prev;
          return prev.slice(0, idx);
        });
        setPendingMessageDelete(null);
        toast.success("Message deleted");
      },
      onError: (err) =>
        toast.error(err instanceof ApiError ? err.message : "Failed to delete message"),
    });
  }

  function decide(approved: boolean) {
    if (selectedId === null || pending.length === 0) return;
    const decisions = pending.map((p) => ({ callId: p.callId, approved, reason: approved ? undefined : "user denied" }));
    resolve(selectedId, decisions).catch((err) =>
      setSendError(err instanceof ApiError ? err.message : "Failed to resolve approvals"),
    );
  }

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

  const activeConversation =
    conversations?.find((c) => c.id === selectedId) ??
    (detail?.conversation.id === selectedId ? detail.conversation : undefined);
  const running = status === "running";

  const activeSettings: ConversationSettings = activeConversation
    ? {
        model: activeConversation.model || models?.default || "",
        temperature: activeConversation.temperature,
        systemPrompt: activeConversation.systemPrompt,
        ragEnabled: activeConversation.ragEnabled,
      }
    : draftSettings;

  function exportConversation() {
    if (!activeConversation || messages.length === 0) return;
    downloadMarkdown(conversationFilename(activeConversation.title), toMarkdown(activeConversation, messages));
    toast.success("Conversation exported");
  }

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

  const startNewChatRef = useRef(startNewChat);
  const regenerateLastRef = useRef(regenerateLast);
  const stopRef = useRef(stop);
  const runningRef = useRef(running);

  useEffect(() => {
    startNewChatRef.current = startNewChat;
    regenerateLastRef.current = regenerateLast;
    stopRef.current = stop;
    runningRef.current = running;
  });

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((o) => !o);
        return;
      }
      if ((e.metaKey || e.ctrlKey) && e.key === "/") {
        e.preventDefault();
        setShortcutsOpen((o) => !o);
        return;
      }
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === "o") {
        e.preventDefault();
        startNewChatRef.current();
        return;
      }
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === "r") {
        e.preventDefault();
        regenerateLastRef.current();
        return;
      }
      if (e.key === "Escape" && runningRef.current) {
        stopRef.current();
        return;
      }

      const target = e.target as HTMLElement | null;
      const tag = target?.tagName;
      const isEditable = target?.isContentEditable;
      if (tag === "INPUT" || tag === "TEXTAREA" || isEditable) {
        return;
      }

      if (e.key === "?") {
        e.preventDefault();
        setShortcutsOpen(true);
      } else if (e.key === "/") {
        e.preventDefault();
        textareaRef.current?.focus();
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  return (
    <div className="flex h-full flex-col bg-background">
      {/* ChatGPT / Codex style Header */}
      <header className="flex items-center justify-between gap-2 border-b border-border/60 px-3 py-2 bg-background/80 backdrop-blur-xs z-10 shrink-0">
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

          {/* Model Selector Pill */}
          <button
            type="button"
            onClick={() => setSettingsOpen(true)}
            className="flex items-center gap-1.5 rounded-lg border border-border/60 bg-muted/30 px-2.5 py-1 text-xs font-medium text-foreground transition-all hover:bg-accent hover:border-foreground/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            title="Model & chat configuration"
          >
            <Bot className="size-3.5 text-primary" />
            <span className="font-semibold">{modelLabel(activeSettings.model)}</span>
            <ChevronDown className="size-3 text-muted-foreground" />
          </button>

          <span className="hidden sm:inline text-muted-foreground/30">|</span>

          <h1 className="min-w-0 truncate text-xs font-medium text-muted-foreground">
            {activeConversation ? cleanConversationTitle(activeConversation.title) : (selectedId ? "Conversation" : "New chat")}
          </h1>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Share conversation"
            title="Share conversation link"
            disabled={!activeConversation}
            onClick={() => activeConversation && setSharing(activeConversation)}
            className="text-muted-foreground hover:text-foreground"
          >
            <Share2 className="size-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Export conversation"
            title="Export conversation as Markdown"
            disabled={!activeConversation || messages.length === 0}
            onClick={exportConversation}
            className="text-muted-foreground hover:text-foreground"
          >
            <Download className="size-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Conversation settings"
            title="Conversation settings"
            onClick={() => setSettingsOpen(true)}
            className="text-muted-foreground hover:text-foreground"
          >
            <SlidersHorizontal className="size-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Keyboard shortcuts"
            title="Keyboard shortcuts (?)"
            onClick={() => setShortcutsOpen(true)}
            className="text-muted-foreground hover:text-foreground"
          >
            <HelpCircle className="size-3.5" />
          </Button>
        </div>
      </header>

      {/* Main message area */}
      <ScrollArea className="min-h-0 flex-1">
        <div className="mx-auto w-full max-w-3xl px-4 py-6">
          {messages.length === 0 && !detailLoading ? (
            <div className="flex min-h-[55vh] flex-col items-center justify-center gap-6 text-center">
              <div className="space-y-2">
                <HarveyAvatar className="mx-auto size-14 drop-shadow-sm" />
                <h2 className="text-2xl font-semibold tracking-tight text-foreground">
                  What's on your mind today?
                </h2>
                <p className="max-w-md text-balance text-muted-foreground text-xs">
                  Ask anything{tools && tools.length > 0 ? ` · ${tools.length} ${tools.length === 1 ? "tool" : "tools"} ready` : ""} · Search documents · Execute code
                </p>
              </div>

              <div className="grid w-full max-w-lg gap-2 sm:grid-cols-2">
                {SUGGESTIONS.map((s, i) => (
                  <button
                    key={s.label}
                    type="button"
                    onClick={() => void submit(s.label)}
                    style={{ animationDelay: `${i * 50}ms` }}
                    className="group flex items-center gap-3 rounded-xl border border-border/60 bg-card p-3 text-left text-xs shadow-xs transition-all duration-200
                      hover:-translate-y-0.5 hover:border-foreground/30 hover:shadow-sm
                      active:translate-y-0 active:scale-[0.99]
                      focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring
                      animate-in fade-in slide-in-from-bottom-2 duration-300 [animation-fill-mode:backwards]"
                  >
                    <span className="flex size-7 shrink-0 items-center justify-center rounded-lg border border-border/60 bg-muted/40 text-foreground/80 group-hover:text-primary group-hover:border-primary/40 transition-colors">
                      <s.icon className="size-3.5" />
                    </span>
                    <span className="min-w-0 text-muted-foreground group-hover:text-foreground transition-colors font-medium">
                      {s.label}
                    </span>
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
                          <div className="flex justify-end gap-1.5 pt-1">
                            <Button size="xs" variant="outline" onClick={() => setEditing(null)}>
                              Cancel
                            </Button>
                            <Button size="xs" disabled={!editing.draft.trim() || blockInteraction} onClick={() => saveEdit(m.id)}>
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
                          {/* Attached files badge and clean prompt in user message */}
                          {(() => {
                            const { filenames, files, userPrompt } = extractFilesAndPrompt(m.content);
                            return (
                              <>
                                {files.length > 0 ? (
                                  <div className="flex flex-wrap justify-end gap-1.5 mb-1">
                                    {files.map((file, idx) => (
                                      <button
                                        key={idx}
                                        type="button"
                                        onClick={() =>
                                          setViewingFile({
                                            title: file.name,
                                            content: file.content,
                                          })
                                        }
                                        className="group/file flex items-center gap-1.5 rounded-xl border border-border/80 bg-muted/50 hover:bg-muted px-2.5 py-1 text-xs transition-colors cursor-pointer"
                                        title="Click to view parsed file"
                                      >
                                        <FileCode className="size-3.5 text-primary" />
                                        <span className="font-medium font-mono text-[11px]">{file.name}</span>
                                        <Eye className="size-3 text-muted-foreground opacity-60 group-hover/file:opacity-100 group-hover/file:text-primary transition-opacity" />
                                      </button>
                                    ))}
                                  </div>
                                ) : filenames.length > 0 ? (
                                  <div className="flex flex-wrap justify-end gap-1.5 mb-1">
                                    {filenames.map((fname, idx) => (
                                      <div key={idx} className="flex items-center gap-1.5 rounded-xl border border-border/80 bg-muted/50 px-2.5 py-1 text-xs">
                                        <FileCode className="size-3.5 text-primary" />
                                        <span className="font-medium font-mono text-[11px]">{fname}</span>
                                      </div>
                                    ))}
                                  </div>
                                ) : null}
                                {(userPrompt || filenames.length > 0) && (
                                  <div className="rounded-2xl rounded-br-md bg-primary px-4 py-2.5 whitespace-pre-wrap text-primary-foreground text-sm shadow-sm">
                                    {userPrompt || (filenames.length === 1 ? `Attached ${filenames[0]}` : `Attached ${filenames.length} files`)}
                                  </div>
                                )}
                              </>
                            );
                          })()}
                          <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <button
                              type="button"
                              aria-label="Edit message"
                              title="Edit & resend"
                              disabled={blockInteraction}
                              onClick={() => setEditing({ id: m.id, draft: m.content })}
                              className="rounded-md p-1 text-muted-foreground/60 hover:text-foreground disabled:cursor-not-allowed"
                            >
                              <Pencil className="size-3.5" />
                            </button>
                            <button
                              type="button"
                              aria-label="Delete message"
                              title="Delete message and everything after it"
                              disabled={blockInteraction}
                              onClick={() => setPendingMessageDelete(m.id)}
                              className="rounded-md p-1 text-muted-foreground/60 hover:text-destructive disabled:cursor-not-allowed"
                            >
                              <Trash2 className="size-3.5" />
                            </button>
                          </div>
                        </div>
                      )
                    ) : (
                      <>
                        <HarveyAvatar isProcessing={m.streaming} className="mt-0.5 size-7.5 shrink-0" />
                        <div className="min-w-0 flex-1 space-y-1 pt-0.5">
                          {m.streaming && trace.length > 0 && <ThinkingTrace rows={trace} />}
                          {m.streaming && m.content === "" ? (
                            <RunLoader trace={trace} />
                          ) : (
                            <StreamingText
                              content={m.content}
                              streaming={m.streaming}
                              citations={
                                m.sources && m.sources.length > 0
                                  ? {
                                      sources: m.sources,
                                      // Citation click opens the indexed source document.
                                      onCite: (n) => {
                                        const s = m.sources?.[n - 1];
                                        if (s) setViewingFile({ title: s.title, documentId: s.documentId });
                                      },
                                    }
                                  : undefined
                              }
                            />
                          )}
                          {!m.streaming && (() => {
                            const connectProviders = extractAppConnectProviders(m.content);
                            if (connectProviders.length === 0) return null;
                            return (
                              <div className="space-y-2 pt-1">
                                {connectProviders.map((p) => (
                                  <AppConnectCard key={p} providerId={p} returnTo={window.location.pathname} />
                                ))}
                              </div>
                            );
                          })()}
                          {m.error && <p className="text-destructive text-sm">{m.error}</p>}
                          {m.truncated && (
                            <Badge variant="outline" className="text-muted-foreground text-xs">stopped early</Badge>
                          )}
                          {!m.streaming && (
                            <div className="flex items-center gap-1.5 pt-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
                              <CopyMessageButton text={m.content} />
                              <button
                                type="button"
                                aria-label="Delete message"
                                title="Delete message and everything after it"
                                disabled={blockInteraction}
                                onClick={() => setPendingMessageDelete(m.id)}
                                className="rounded-md p-1 text-muted-foreground/60 transition-colors hover:text-destructive disabled:cursor-not-allowed"
                              >
                                <Trash2 className="size-3.5" />
                              </button>
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

      {/* ChatGPT / Codex style Floating Composer */}
      <div className="p-3 md:p-4 bg-gradient-to-t from-background via-background to-transparent shrink-0">
        <div className="mx-auto w-full max-w-3xl">
          {sendError && <p className="mb-2 text-destructive text-xs">{sendError}</p>}
          {attachments.length > 0 && (
            <div className="mb-2 flex flex-wrap gap-2">
              {attachments.map((a) => (
                <div key={a.previewUrl} className="group/img relative">
                  <img src={a.previewUrl} alt={a.name} className="size-16 rounded-xl border border-border/80 object-cover shadow-xs" />
                  <button
                    type="button"
                    aria-label={`Remove ${a.name}`}
                    onClick={() => removeAttachment(a.previewUrl)}
                    className="absolute -top-1.5 -right-1.5 rounded-full border bg-background p-0.5 text-muted-foreground shadow-sm hover:text-destructive"
                  >
                    <X className="size-3" />
                  </button>
                </div>
              ))}
            </div>
          )}

          {attachedFiles.length > 0 && (
            <div className="mb-2 flex flex-wrap gap-2">
              {attachedFiles.map((doc) => (
                <div
                  key={doc.id}
                  className="flex items-center gap-1.5 rounded-xl border border-border/80 bg-muted/50 px-2.5 py-1 text-xs shadow-xs"
                >
                  <FileCode className="size-3.5 text-primary shrink-0" />
                  <span className="font-medium truncate max-w-[160px]">{doc.name}</span>
                  <span className="text-[10px] text-muted-foreground font-mono">
                    ({Math.ceil(doc.size / 1024)} KB)
                  </span>
                  <button
                    type="button"
                    aria-label={`View parsed content of ${doc.name}`}
                    title="View parsed content"
                    onClick={() =>
                      setViewingFile({
                        title: doc.name,
                        content: doc.content,
                        sizeBytes: doc.size,
                      })
                    }
                    className="ml-1 rounded-full p-0.5 text-muted-foreground hover:text-foreground hover:bg-muted"
                  >
                    <Eye className="size-3" />
                  </button>
                  <button
                    type="button"
                    aria-label={`Remove ${doc.name}`}
                    onClick={() => removeAttachedFile(doc.id)}
                    className="rounded-full p-0.5 text-muted-foreground hover:text-destructive hover:bg-muted"
                  >
                    <X className="size-3" />
                  </button>
                </div>
              ))}
            </div>
          )}

          {extractingFiles && (
            <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground bg-muted/40 px-3 py-1.5 rounded-xl">
              <Loader2 className="size-3.5 animate-spin text-primary" />
              <span>Reading file content…</span>
            </div>
          )}

          <div
            className={cn(
              "relative rounded-2xl border border-border/80 bg-card/95 backdrop-blur-md shadow-md transition-all",
              "focus-within:border-ring/80 focus-within:ring-[3px] focus-within:ring-ring/20 focus-within:shadow-lg",
              pending.length > 0 && "opacity-60",
            )}
          >
            {mentionQuery !== null && (
              <MentionMenu
                apps={filteredApps}
                selectedIndex={mentionSelectedIndex}
                onSelect={handleSelectApp}
                onHoverIndex={setMentionSelectedIndex}
                onClose={() => setMentionQuery(null)}
              />
            )}
            <div className="p-2.5 pb-1">
              <textarea
                ref={textareaRef}
                value={input}
                onChange={handleInputChange}
                onKeyDown={handleInputKeyDown}
                rows={1}
                placeholder={pending.length > 0 ? "Waiting for approval…" : "Message Herbie… (type @ for apps & tools)"}
                disabled={running || pending.length > 0}
                className="max-h-48 min-h-9 w-full resize-none bg-transparent px-2 py-1 text-sm outline-none placeholder:text-muted-foreground/60 field-sizing-content disabled:cursor-not-allowed"
              />
            </div>

            <div className="flex items-center justify-between px-2.5 pb-2 pt-1 border-t border-border/30">
              <div className="flex items-center gap-1">
                <input
                  ref={fileInputRef}
                  type="file"
                  accept="image/png,image/jpeg,image/webp,image/gif,.pdf,.docx,.xlsx,.pptx,.txt,.md,.json,.csv,.tsv,.py,.js,.ts,.tsx,.jsx,.go,.rs,.sh,.sql,.yaml,.yml"
                  multiple
                  hidden
                  onChange={(e) => void pickFiles(e.target.files)}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  className="rounded-lg text-muted-foreground hover:text-foreground"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={running || pending.length > 0 || extractingFiles}
                  aria-label="Attach files or images"
                  title="Attach files (images, documents, code)"
                >
                  <Paperclip className="size-3.5" />
                </Button>

                <Button
                  type="button"
                  variant="ghost"
                  size="xs"
                  onClick={() => setSettingsOpen(true)}
                  className="rounded-lg text-[11px] text-muted-foreground hover:text-foreground gap-1 px-2 h-7"
                  title="Model & Prompt Settings"
                >
                  <Bot className="size-3 text-primary" />
                  <span className="hidden sm:inline font-mono">{modelLabel(activeSettings.model)}</span>
                </Button>

                {detectedApp && (
                  <Badge
                    variant="outline"
                    className="gap-1 border-primary/30 bg-primary/10 text-primary text-[11px] h-6 px-2 font-normal animate-in fade-in zoom-in-95 duration-150"
                  >
                    <MentionAppIcon name={detectedApp.iconName} className="size-3" />
                    <span>@{detectedApp.mention}</span>
                  </Badge>
                )}
              </div>

              <div className="flex items-center gap-1.5">
                {voice.supported && (
                  <Button
                    size="icon-xs"
                    variant={voice.listening ? "default" : "ghost"}
                    className={cn(
                      "rounded-lg text-muted-foreground hover:text-foreground",
                      voice.listening && "bg-rose-500 text-white animate-pulse"
                    )}
                    onClick={toggleVoice}
                    disabled={running || pending.length > 0}
                    aria-label={voice.listening ? "Stop dictation" : "Start dictation"}
                    title={voice.listening ? "Stop dictation" : "Dictate"}
                  >
                    {voice.listening ? <Square className="size-3" /> : <Mic className="size-3.5" />}
                  </Button>
                )}

                {running ? (
                  <Button
                    size="icon-xs"
                    variant="default"
                    className="rounded-lg size-7"
                    onClick={stop}
                    aria-label="Stop generating"
                    title="Stop generating"
                  >
                    <Square className="size-3 fill-current" />
                  </Button>
                ) : (
                  <Button
                    size="icon-xs"
                    variant="brand"
                    className="rounded-lg size-7"
                    onClick={() => void submit()}
                    disabled={(!input.trim() && attachments.length === 0) || createConversation.isPending}
                    aria-label="Send message"
                    title="Send message"
                  >
                    <ArrowUp className="size-3.5" />
                  </Button>
                )}
              </div>
            </div>
          </div>

          <p className="px-1 pt-1.5 text-center text-[11px] text-muted-foreground/60">
            {pending.length > 0
              ? "Resolve the approval above to continue."
              : "Herbie can make mistakes. Consider checking important information."}
          </p>
        </div>
      </div>

      <ConversationSettingsDialog
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        models={models}
        settings={activeSettings}
        onApply={applySettings}
        saving={updateSettings.isPending}
      />

      <ShareDialog conversation={sharing} onOpenChange={(open) => { if (!open) setSharing(null); }} />

      <Dialog open={pendingMessageDelete !== null} onOpenChange={(open) => { if (!open) setPendingMessageDelete(null); }}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Delete message?</DialogTitle>
            <DialogDescription>
              This message and everything after it will be permanently removed.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingMessageDelete(null)}>Cancel</Button>
            <Button variant="destructive" onClick={confirmDeleteMessage} disabled={deleteMessage.isPending}>
              {deleteMessage.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <CommandPalette
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        conversations={conversations}
        activeConversation={activeConversation}
        onNewChat={startNewChat}
        onExport={exportConversation}
        onShare={() => activeConversation && setSharing(activeConversation)}
        onOpenSettings={() => setSettingsOpen(true)}
        onOpenShortcuts={() => setShortcutsOpen(true)}
      />

      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />

      {/* Parsed File Viewer Dialog */}
      <ParsedFileViewerDialog
        open={Boolean(viewingFile)}
        onOpenChange={(open) => {
          if (!open) setViewingFile(null);
        }}
        title={viewingFile?.title || ""}
        documentId={viewingFile?.documentId}
        content={viewingFile?.content}
        sizeBytes={viewingFile?.sizeBytes}
        status={viewingFile?.status}
        chunkCount={viewingFile?.chunkCount}
      />
    </div>
  );
}
