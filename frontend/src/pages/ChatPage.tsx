import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArchiveRestore,
  ArrowUp,
  BookMarked,
  BookOpen,
  Bot,
  Calendar,
  Check,
  ChevronDown,
  ChevronUp,
  Copy,
  Download,
  Eye,
  FileCode,
  GitFork,
  Globe,
  HelpCircle,
  Loader2,
  Mic,
  Paperclip,
  Pencil,
  PenLine,
  RefreshCw,
  Reply,
  ScrollText,
  Search,
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
import { BranchSwitcher } from "@/features/chat/BranchSwitcher";
import { FeedbackButtons } from "@/features/chat/FeedbackButtons";
import { findMessageMatches } from "@/features/chat/findInConversation";
import { CommandPalette } from "@/features/chat/CommandPalette";
import { PromptLibraryDialog } from "@/features/chat/PromptLibraryDialog";
import { extractCitationsFromContent } from "@/features/chat/citations";
import { ConversationSettingsDialog } from "@/features/chat/ConversationSettingsDialog";
import { conversationFilename, downloadMarkdown, toMarkdown } from "@/features/chat/exportMarkdown";
import { ShareDialog } from "@/features/chat/ShareDialog";
import { SummaryDialog } from "@/features/chat/SummaryDialog";
import { ShortcutsDialog } from "@/features/chat/ShortcutsDialog";
import { useChat } from "@/features/chat/useChat";
import { useModels } from "@/features/chat/useModels";
import { useVoiceInput } from "@/features/chat/useVoiceInput";
import {
  useActivateBranch,
  useArchiveConversation,
  useConversations,
  useCreateConversation,
  useDeleteMessage,
  useForkConversation,
  useUpdateConversationSettings,
} from "@/features/chat/useConversations";
import { useTools } from "@/features/tools/useTools";
import { useDocuments } from "@/features/documents/useDocuments";
import { ContextStatusMeter } from "@/features/chat/ContextStatusMeter";

interface ConversationDetail {
  conversation: Conversation;
  messages: ChatMessage[];
  context?: {
    estimatedTokens: number;
    thresholdTokens: number;
    keepRecent?: number;
    compacted?: boolean;
  };
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
    branch: m.branch,
    replyToId: m.replyToId,
    quoteText: m.quoteText,
    feedback: m.feedback,
  };
}

function mutationErrorMessage(err: unknown, fallback: string): string | null {
  if (err instanceof ApiError && err.status === 409) {
    if (err.code === "run_active") return "A run is in progress — wait for it to finish";
    if (err.code === "approval_pending") return "Resolve the pending approval first";
    if (err.code === "bot_conversation") return null;
  }
  return err instanceof ApiError ? err.message : fallback;
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
  const [summaryOpen, setSummaryOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [promptLibraryOpen, setPromptLibraryOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const [editing, setEditing] = useState<{ id: string; draft: string } | null>(null);
  const [replyTarget, setReplyTarget] = useState<{
    conversationId: string;
    id: string;
    content: string;
  } | null>(null);
  const [findOpen, setFindOpen] = useState(false);
  const [findQuery, setFindQuery] = useState("");
  const [findIndex, setFindIndex] = useState(0);
  const [flashMessageId, setFlashMessageId] = useState<string | null>(null);
  const flashTimerRef = useRef<number | null>(null);
  const attemptedJumpRef = useRef<string | null>(null);
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

  const insertPrompt = (body: string) => {
    const textarea = textareaRef.current;
    if (!textarea) {
      setInput((prev) => (prev ? `${prev}\n${body}` : body));
      return;
    }
    const start = textarea.selectionStart ?? input.length;
    const end = textarea.selectionEnd ?? start;
    const next = input.slice(0, start) + body + input.slice(end);
    setInput(next);
    setTimeout(() => {
      textarea.focus();
      const pos = start + body.length;
      textarea.setSelectionRange(pos, pos);
    }, 0);
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
    topP: null,
    maxOutputTokens: 0,
    reasoning: "",
    systemPrompt: "",
    ragEnabled: false,
  });

  useEffect(() => {
    setDraftSettings((prev) => ({ ...prev, ragEnabled: hasDocs }));
  }, [hasDocs]);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const seededRef = useRef<string | null>(null);
  const selectedIdRef = useRef(selectedId);

  useEffect(() => {
    selectedIdRef.current = selectedId;
  }, [selectedId]);

  const { messages, setMessages, status, send, edit, regenerate, resolve, stop, reset, trace, pending, contextStatus, setContextStatus } = useChat(() => {
    void queryClient.invalidateQueries({ queryKey: ["conversations"] });
    const id = selectedIdRef.current;
    if (id !== null) void queryClient.invalidateQueries({ queryKey: ["conversation", id] });
  });

  const { data: conversations } = useConversations();
  const { data: tools } = useTools();
  const { data: models } = useModels();
  const createConversation = useCreateConversation();
  const deleteMessage = useDeleteMessage(selectedId);
  const activateBranch = useActivateBranch(selectedId);
  const forkConversation = useForkConversation(selectedId);
  const updateSettings = useUpdateConversationSettings(selectedId);
  const archiveConversation = useArchiveConversation();

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

  // Reset only when the selection itself becomes empty. Depending on status
  // here would abort an in-flight send that starts before the router commits
  // the new conversation id (the /chat -> /chat/:id transition).
  useEffect(() => {
    if (selectedId !== null) return;
    seededRef.current = null;
    reset();
  }, [selectedId, reset]);

  useEffect(() => {
    if (selectedId === null || !detail || status === "running") return;
    const tailId = detail.messages.at(-1)?.id ?? "";
    const seedKey = `${selectedId}:${detail.conversation.activeLeafId ?? ""}:${tailId}`;
    if (seededRef.current === seedKey) return;
    seededRef.current = seedKey;
    setMessages(detail.messages.map(toChatMessage));
    if (detail.context) {
      setContextStatus(detail.context);
    }
    setSendError(null);
  }, [selectedId, detail, status, setMessages, setContextStatus]);

  function scrollToMessage(messageId: string) {
    const el = document.getElementById(`message-${messageId}`);
    if (!el) return;
    el.scrollIntoView({ behavior: "smooth", block: "center" });
    setFlashMessageId(messageId);
    if (flashTimerRef.current) window.clearTimeout(flashTimerRef.current);
    flashTimerRef.current = window.setTimeout(() => setFlashMessageId(null), 2000);
  }

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, trace, pending]);

  // `?m=<messageId>` is the search-result jump. If the row is off the active
  // path, activate its branch once and let the refetch bring it in.
  const jumpId = searchParams.get("m");
  useEffect(() => {
    if (!jumpId || selectedId === null || !detail || status === "running") return;
    if (messages.some((m) => m.id === jumpId)) {
      scrollToMessage(jumpId);
      const next = new URLSearchParams(searchParams);
      next.delete("m");
      setSearchParams(next, { replace: true });
      return;
    }
    if (attemptedJumpRef.current === jumpId) return;
    attemptedJumpRef.current = jumpId;
    void apiFetch<void>(`/api/conversations/${selectedId}/messages/${jumpId}/activate`, { method: "POST" })
      .then(() => queryClient.invalidateQueries({ queryKey: ["conversation", selectedId] }))
      .catch(() => {
        const next = new URLSearchParams(searchParams);
        next.delete("m");
        setSearchParams(next, { replace: true });
      });
  }, [jumpId, messages, detail, status, selectedId, searchParams, setSearchParams, queryClient]);

  const findMatches = useMemo(() => findMessageMatches(messages, findQuery), [messages, findQuery]);
  const findMatchId = findOpen ? findMatches[Math.min(findIndex, Math.max(findMatches.length - 1, 0))] : undefined;

  useEffect(() => {
    if (!findOpen || status === "running" || findMatches.length === 0) return;
    scrollToMessage(findMatches[Math.min(findIndex, findMatches.length - 1)]);
  }, [findOpen, findQuery, findMatches, findIndex, status]);

  function stepFind(delta: number) {
    if (findMatches.length === 0) return;
    const next = (findIndex + delta + findMatches.length) % findMatches.length;
    setFindIndex(next);
  }

  function startNewChat() {
    navigate("/chat");
    reset();
    setInput("");
    setAttachments([]);
    setAttachedFiles([]);
    setEditing(null);
    setReplyTarget(null);
    setFindOpen(false);
    setFindQuery("");
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
    const reply = replyTarget && replyTarget.conversationId === selectedId ? replyTarget : null;
    if (!overrideText) {
      setInput("");
      setAttachments([]);
      setAttachedFiles([]);
    }
    setReplyTarget(null);

    try {
      let conversationId = selectedId;
      if (conversationId === null) {
        const conv = await createConversation.mutateAsync(draftSettings);
        conversationId = conv.id;
        navigate(`/chat/${conversationId}`, { replace: true });
      }
      await send(outgoingText, conversationId, images, reply?.id, reply?.content);
    } catch (err) {
      setSendError(err instanceof ApiError ? err.message : "Failed to send message");
    }
  }

  function regenerateLast(model?: string) {
    if (selectedId === null || running || pending.length > 0) return;
    setSendError(null);
    regenerate(selectedId, model).catch((err) =>
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
        toast.success("Branch deleted");
      },
      onError: (err) => {
        const message = mutationErrorMessage(err, "Failed to delete message");
        if (message) toast.error(message);
      },
    });
  }

  function activateBranchMessage(messageId: string) {
    if (selectedId === null || running || pending.length > 0) return;
    activateBranch.mutate(messageId, {
      onError: (err) => {
        const message = mutationErrorMessage(err, "Failed to switch branch");
        if (message) toast.error(message);
      },
    });
  }

  function forkCurrentConversation() {
    if (selectedId === null || running || pending.length > 0) return;
    forkConversation.mutate(undefined, {
      onSuccess: (conversation) => navigate(`/chat/${conversation.id}`),
      onError: (err) => {
        const message = mutationErrorMessage(err, "Failed to fork conversation");
        if (message) toast.error(message);
      },
    });
  }

  function decide(approved: boolean) {
    if (selectedId === null || pending.length === 0) return;
    const decisions = pending.map((p) => ({ callId: p.callId, approved: true, reason: approved ? undefined : "user denied" }));
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

  const replyingTo =
    replyTarget && replyTarget.conversationId === selectedId ? replyTarget : null;
  const activeConversation =
    conversations?.find((c) => c.id === selectedId) ??
    (detail?.conversation.id === selectedId ? detail.conversation : undefined);
  const running = status === "running";
  const isBotConversation = Boolean(detail?.conversation.botId ?? activeConversation?.botId);

  const activeEstimatedTokens = contextStatus?.estimatedTokens ?? messages.reduce((acc, m) => acc + Math.round(m.content.length / 4) + 40, 0);
  const activeThresholdTokens = contextStatus?.thresholdTokens ?? 40000;
  const activeKeepRecent = contextStatus?.keepRecent ?? 10;
  const isCompacted = contextStatus?.compacted ?? false;

  const activeSettings: ConversationSettings = activeConversation
    ? {
        model: activeConversation.model || models?.default || "",
        temperature: activeConversation.temperature,
        topP: activeConversation.topP ?? null,
        maxOutputTokens: activeConversation.maxOutputTokens ?? 0,
        reasoning: activeConversation.reasoning ?? "",
        systemPrompt: activeConversation.systemPrompt,
        ragEnabled: activeConversation.ragEnabled,
      }
    : draftSettings;

  function switchModel(modelID: string) {
    if (modelID === activeSettings.model) return;
    if (selectedId === null) {
      setDraftSettings((prev) => ({ ...prev, model: modelID }));
      return;
    }
    updateSettings.mutate(
      { ...activeSettings, model: modelID },
      { onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to switch model") },
    );
  }

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
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "f") {
        e.preventDefault();
        setFindOpen(true);
        return;
      }
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

          {/* Inline model switcher */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                disabled={running || pending.length > 0}
                className="flex items-center gap-1.5 rounded-lg border border-border/60 bg-muted/30 px-2.5 py-1 text-xs font-medium text-foreground transition-all hover:bg-accent hover:border-foreground/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                title="Switch model"
              >
                <Bot className="size-3.5 text-primary" />
                <span className="font-semibold">{modelLabel(activeSettings.model)}</span>
                <ChevronDown className="size-3 text-muted-foreground" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="max-h-80 w-56 overflow-y-auto">
              <DropdownMenuLabel>Model</DropdownMenuLabel>
              {(models?.models ?? []).map((m) => (
                <DropdownMenuItem key={m.id} onClick={() => switchModel(m.id)} className="justify-between">
                  <span>{m.label}</span>
                  {m.id === activeSettings.model && <Check className="size-3.5 text-primary" />}
                </DropdownMenuItem>
              ))}
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setSettingsOpen(true)}>
                Model & prompt settings…
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>

          <span className="hidden sm:inline text-muted-foreground/30">|</span>

          <h1 className="min-w-0 truncate text-xs font-medium text-muted-foreground">
            {activeConversation ? cleanConversationTitle(activeConversation.title) : (selectedId ? "Conversation" : "New chat")}
          </h1>
          {activeConversation?.archivedAt && (
            <span className="flex shrink-0 items-center gap-1">
              <Badge variant="outline" className="h-5 px-1.5 text-[10px] font-normal">Archived</Badge>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label="Unarchive conversation"
                title="Unarchive conversation"
                className="text-muted-foreground hover:text-foreground"
                onClick={() =>
                  archiveConversation.mutate(
                    { id: activeConversation.id, archived: false },
                    {
                      onSuccess: () => toast.success("Conversation unarchived"),
                      onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to unarchive"),
                    },
                  )
                }
              >
                <ArchiveRestore className="size-3.5" />
              </Button>
            </span>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-1">
          {!isBotConversation && (
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label="Fork conversation"
              title="Fork conversation"
              disabled={!activeConversation || running || pending.length > 0 || forkConversation.isPending}
              onClick={forkCurrentConversation}
              className="text-muted-foreground hover:text-foreground"
            >
              <GitFork className="size-3.5" />
            </Button>
          )}
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

      {findOpen && (
        <div className="flex shrink-0 items-center gap-2 border-b border-border/60 bg-muted/30 px-3 py-1.5">
          <Search className="size-3.5 shrink-0 text-muted-foreground" />
          <input
            autoFocus
            value={findQuery}
            onChange={(e) => {
              setFindQuery(e.target.value);
              setFindIndex(0);
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                stepFind(e.shiftKey ? -1 : 1);
              } else if (e.key === "Escape") {
                e.preventDefault();
                setFindOpen(false);
                setFindQuery("");
              }
            }}
            placeholder="Find in conversation"
            aria-label="Find in conversation"
            className="h-7 flex-1 bg-transparent text-xs outline-none placeholder:text-muted-foreground/60"
          />
          <span className="text-[11px] text-muted-foreground">
            {findQuery.trim().length >= 2
              ? findMatches.length === 0
                ? "0/0"
                : `${Math.min(findIndex + 1, findMatches.length)}/${findMatches.length}`
              : ""}
          </span>
          <button
            type="button"
            aria-label="Previous match"
            title="Previous match (Shift+Enter)"
            disabled={findMatches.length === 0}
            onClick={() => stepFind(-1)}
            className="rounded-md p-1 text-muted-foreground/60 hover:text-foreground disabled:opacity-40"
          >
            <ChevronUp className="size-3.5" />
          </button>
          <button
            type="button"
            aria-label="Next match"
            title="Next match (Enter)"
            disabled={findMatches.length === 0}
            onClick={() => stepFind(1)}
            className="rounded-md p-1 text-muted-foreground/60 hover:text-foreground disabled:opacity-40"
          >
            <ChevronDown className="size-3.5" />
          </button>
          <button
            type="button"
            aria-label="Close find"
            title="Close find (Esc)"
            onClick={() => {
              setFindOpen(false);
              setFindQuery("");
            }}
            className="rounded-md p-1 text-muted-foreground/60 hover:text-foreground"
          >
            <X className="size-3.5" />
          </button>
        </div>
      )}

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
                  <div
                    key={m.id}
                    id={`message-${m.id}`}
                    className={cn(
                      "flex scroll-mt-4",
                      m.role === "user" ? "justify-end" : "group gap-3",
                      findMatchId === m.id && "rounded-2xl bg-primary/5",
                      flashMessageId === m.id && "rounded-2xl ring-2 ring-primary/50",
                    )}
                  >
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
                          {m.quoteText && (
                            <div className="max-w-full rounded-xl border-l-2 border-primary/50 bg-muted/50 px-3 py-1.5 text-left text-xs text-muted-foreground">
                              <span className="block text-[10px] font-medium uppercase tracking-wide text-muted-foreground/70">
                                Replying to
                              </span>
                              <span className="line-clamp-3 whitespace-pre-wrap">{m.quoteText}</span>
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
                            {!isBotConversation && m.branch && (
                              <BranchSwitcher
                                branch={m.branch}
                                disabled={blockInteraction || activateBranch.isPending}
                                onActivate={activateBranchMessage}
                              />
                            )}
                            <button
                              type="button"
                              aria-label="Reply to message"
                              title="Reply"
                              disabled={blockInteraction}
                              onClick={() => {
                                if (selectedId) setReplyTarget({ conversationId: selectedId, id: m.id, content: m.content });
                              }}
                              className="rounded-md p-1 text-muted-foreground/60 hover:text-foreground disabled:cursor-not-allowed"
                            >
                              <Reply className="size-3.5" />
                            </button>
                            {!isBotConversation && (
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
                            )}
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
                          ) : (() => {
                            const activeSources =
                              m.sources && m.sources.length > 0
                                ? m.sources
                                : extractCitationsFromContent(m.content);
                            return (
                              <StreamingText
                                content={m.content}
                                streaming={m.streaming}
                                citations={
                                  activeSources.length > 0
                                    ? {
                                        sources: activeSources,
                                        // Citation click opens the indexed source document or web link.
                                        onCite: (n) => {
                                          const s = activeSources[n - 1];
                                          if (s) {
                                            const url =
                                              s.url ||
                                              (s.documentId?.startsWith("http://") || s.documentId?.startsWith("https://")
                                                ? s.documentId
                                                : undefined) ||
                                              (s.heading?.startsWith("http://") || s.heading?.startsWith("https://")
                                                ? s.heading
                                                : undefined);
                                            if (url) {
                                              window.open(url, "_blank", "noopener,noreferrer");
                                            } else if (s.documentId) {
                                              setViewingFile({ title: s.title, documentId: s.documentId });
                                            }
                                          }
                                        },
                                      }
                                    : undefined
                                }
                              />
                            );
                          })()}
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
                              {!isBotConversation && m.branch && (
                                <BranchSwitcher
                                  branch={m.branch}
                                  disabled={blockInteraction || activateBranch.isPending}
                                  onActivate={activateBranchMessage}
                                />
                              )}
                              <CopyMessageButton text={m.content} />
                              <button
                                type="button"
                                aria-label="Reply to message"
                                title="Reply"
                                disabled={blockInteraction}
                                onClick={() => {
                                  if (selectedId) setReplyTarget({ conversationId: selectedId, id: m.id, content: m.content });
                                }}
                                className="rounded-md p-1 text-muted-foreground/60 transition-colors hover:text-foreground disabled:cursor-not-allowed"
                              >
                                <Reply className="size-3.5" />
                              </button>
                              <FeedbackButtons
                                conversationId={selectedId}
                                messageId={m.id}
                                value={m.feedback}
                                onChange={(rating) =>
                                  setMessages((prev) => prev.map((msg) => (msg.id === m.id ? { ...msg, feedback: rating } : msg)))
                                }
                              />
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
                              {isLast && !isBotConversation && (
                                <DropdownMenu>
                                  <DropdownMenuTrigger asChild>
                                    <button
                                      type="button"
                                      aria-label="Regenerate answer"
                                      title="Regenerate"
                                      disabled={blockInteraction}
                                      className="rounded-md p-1 text-muted-foreground/60 transition-colors hover:text-foreground disabled:cursor-not-allowed"
                                    >
                                      <RefreshCw className="size-3.5" />
                                    </button>
                                  </DropdownMenuTrigger>
                                  <DropdownMenuContent align="end" className="max-h-80 w-56 overflow-y-auto">
                                    <DropdownMenuItem onClick={() => regenerateLast()}>Regenerate</DropdownMenuItem>
                                    <DropdownMenuSeparator />
                                    <DropdownMenuLabel>Regenerate with</DropdownMenuLabel>
                                    {(models?.models ?? []).map((m) => (
                                      <DropdownMenuItem
                                        key={m.id}
                                        onClick={() => regenerateLast(m.id)}
                                        className="justify-between"
                                      >
                                        <span>{m.label}</span>
                                        {m.id === activeSettings.model && <Check className="size-3.5 text-primary" />}
                                      </DropdownMenuItem>
                                    ))}
                                  </DropdownMenuContent>
                                </DropdownMenu>
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
            {replyingTo && (
              <div className="flex items-start gap-2 border-b border-border/40 px-3 py-2">
                <div className="min-w-0 flex-1">
                  <p className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground/70">Replying to</p>
                  <p className="line-clamp-2 whitespace-pre-wrap text-xs text-muted-foreground">{replyingTo.content}</p>
                </div>
                <button
                  type="button"
                  aria-label="Cancel reply"
                  title="Cancel reply"
                  onClick={() => setReplyTarget(null)}
                  className="rounded-md p-1 text-muted-foreground/60 hover:text-foreground"
                >
                  <X className="size-3.5" />
                </button>
              </div>
            )}
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
                  size="icon-xs"
                  className="rounded-lg text-muted-foreground hover:text-foreground"
                  onClick={() => setPromptLibraryOpen(true)}
                  disabled={running || pending.length > 0}
                  aria-label="Prompt library"
                  title="Prompt library"
                >
                  <BookMarked className="size-3.5" />
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

                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  className="rounded-lg text-muted-foreground hover:text-foreground"
                  onClick={() => setSummaryOpen(true)}
                  disabled={!selectedId || running || pending.length > 0}
                  aria-label="Conversation summary"
                  title="Conversation summary"
                >
                  <ScrollText className="size-3.5" />
                </Button>

                <ContextStatusMeter
                  estimatedTokens={activeEstimatedTokens}
                  thresholdTokens={activeThresholdTokens}
                  breakdown={contextStatus?.breakdown}
                  exact={contextStatus?.exact}
                  keepRecent={activeKeepRecent}
                  compacted={isCompacted}
                />

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

      <SummaryDialog
        open={summaryOpen}
        onOpenChange={setSummaryOpen}
        conversationId={selectedId ?? undefined}
      />

      <PromptLibraryDialog
        open={promptLibraryOpen}
        onOpenChange={setPromptLibraryOpen}
        onInsert={insertPrompt}
      />

      <ShareDialog conversation={sharing} onOpenChange={(open) => { if (!open) setSharing(null); }} />

      <Dialog open={pendingMessageDelete !== null} onOpenChange={(open) => { if (!open) setPendingMessageDelete(null); }}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Delete this branch?</DialogTitle>
            <DialogDescription>
              This message and its continuation in this branch will be permanently removed. Other variants stay available.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingMessageDelete(null)}>Cancel</Button>
            <Button
              variant="destructive"
              onClick={confirmDeleteMessage}
              disabled={deleteMessage.isPending || running || pending.length > 0}
            >
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
        onOpenPromptLibrary={() => setPromptLibraryOpen(true)}
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
