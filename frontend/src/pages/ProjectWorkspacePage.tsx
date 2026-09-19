import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams, useParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowLeft,
  ArrowUp,
  Check,
  CheckCircle2,
  ChevronDown,
  Copy,
  Eye,
  FileCode,
  FileText,
  FolderGit2,
  Loader2,
  MessageSquare,
  Mic,
  Paperclip,
  Plus,
  RefreshCw,
  ShieldCheck,
  Sidebar,
  SlidersHorizontal,
  Square,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import { ApiError, apiFetch } from "@/lib/api";
import { cn, cleanConversationTitle, fmtTokens } from "@/lib/utils";
import type { ChatMessage, Conversation } from "@/lib/types";
import { ContextStatusMeter } from "@/features/chat/ContextStatusMeter";
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
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { HarveyAvatar } from "@/components/HarveyAvatar";
import { RunLoader } from "@/components/ai/RunLoader";
import { StreamingText } from "@/components/ai/StreamingText";
import { ThinkingTrace } from "@/components/ai/ThinkingTrace";
import { useSidebar } from "@/components/layout/SidebarContext";
import { useChat } from "@/features/chat/useChat";
import { useVoiceInput } from "@/features/chat/useVoiceInput";
import { useDeleteMessage } from "@/features/chat/useConversations";
import { AppConnectCard } from "@/features/chat/AppConnectCard";
import { extractAppConnectProviders } from "@/features/chat/useChatApps";
import { ParsedFileViewerDialog } from "@/features/documents/ParsedFileViewerDialog";
import {
  useProject,
  useUploadProjectFile,
  useCreateProjectConversation,
} from "@/features/projects/useProjects";
import {
  type AttachedFile,
  MAX_ATTACHED_FILES,
  MAX_ATTACHED_CHARS,
  isSupportedDocOrCodeFile,
  processAttachedFile,
  formatPromptWithFiles,
  extractFilesAndPrompt,
} from "@/lib/files";
import { ALLOWED_IMAGE_TYPES, MAX_IMAGES_PER_MESSAGE, readImageFiles, type PendingImage } from "@/lib/images";

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
        } catch {
          /* clipboard unavailable */
        }
      }}
      className="rounded-md p-1 text-muted-foreground/60 transition-colors hover:text-foreground"
    >
      {copied ? <Check className="size-3.5 text-emerald-600" /> : <Copy className="size-3.5" />}
    </button>
  );
}

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
  };
}

export function ProjectWorkspacePage() {
  const { projectId } = useParams<{ projectId: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  // Deep links from the sidebar: ?c=<conversationId|new> picks the chat,
  // ?f=<fileId> opens a source in the viewer.
  const convParam = searchParams.get("c");
  const fileParam = searchParams.get("f");
  const queryClient = useQueryClient();
  const { toggleSidebar, setMobileOpen } = useSidebar();

  const { data: projectData, isLoading: projectLoading, isError: projectError } = useProject(projectId ?? null);
  const uploadFile = useUploadProjectFile(projectId ?? null);
  const createProjectConv = useCreateProjectConversation(projectId ?? null);

  const [filesOpen, setFilesOpen] = useState(true);
  const [projectFileDragging, setProjectFileDragging] = useState(false);
  const [instructionsOpen, setInstructionsOpen] = useState(false);
  const [selectedConvId, setSelectedConvId] = useState<string | null>(null);

  // Chat states
  const [input, setInput] = useState("");
  const [sendError, setSendError] = useState<string | null>(null);
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
  const projectFileInputRef = useRef<HTMLInputElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const seededRef = useRef<string | null>(null);

  const { messages, setMessages, status, send, regenerate, resolve, stop, reset, trace, pending, contextStatus, setContextStatus } = useChat(() => {
    void queryClient.invalidateQueries({ queryKey: ["project", projectId] });
  });
  const deleteMessage = useDeleteMessage(selectedConvId);
  const [pendingMessageDelete, setPendingMessageDelete] = useState<string | null>(null);

  function regenerateLast() {
    if (!selectedConvId || status === "running" || pending.length > 0) return;
    setSendError(null);
    regenerate(selectedConvId).catch((err) =>
      setSendError(err instanceof ApiError ? err.message : "Failed to regenerate answer"),
    );
  }

  function confirmDeleteMessage() {
    if (!pendingMessageDelete || !selectedConvId) return;
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
    if (!selectedConvId || pending.length === 0) return;
    const decisions = pending.map((p) => ({
      callId: p.callId,
      approved,
      reason: approved ? undefined : "user denied",
    }));
    resolve(selectedConvId, decisions).catch((err) =>
      setSendError(err instanceof ApiError ? err.message : "Failed to resolve approvals"),
    );
  }

  // Switching projects must drop the previous project's selection and chat
  // state, otherwise the detail query would fetch another project's chat.
  useEffect(() => {
    setSelectedConvId(null);
    seededRef.current = null;
    reset();
  }, [projectId, reset]);

  // Sidebar navigation drives the selection: a conversation id selects it,
  // "new" starts a fresh chat (first send creates the conversation).
  useEffect(() => {
    if (convParam === "new") {
      setSelectedConvId(null);
      seededRef.current = null;
      reset();
      setInput("");
      setSendError(null);
      setTimeout(() => textareaRef.current?.focus(), 50);
    } else if (convParam) {
      setSelectedConvId(convParam);
    }
  }, [convParam, reset]);

  // Pick first conversation if available, or create one — but never override
  // an explicit ?c= target.
  useEffect(() => {
    if (projectData && !selectedConvId && !convParam) {
      if (projectData.conversations.length > 0) {
        setSelectedConvId(projectData.conversations[0].id);
      }
    }
  }, [projectData, selectedConvId, convParam]);

  // Open the requested source once the project's files are loaded, then drop
  // the param so clicking the same file again re-triggers the viewer.
  useEffect(() => {
    if (!fileParam) return;
    if (projectData) {
      const f = projectData.files.find((x) => x.id === fileParam);
      if (f) {
        setViewingFile({
          title: f.filename,
          documentId: f.id,
          sizeBytes: f.sizeBytes,
          status: f.status,
          chunkCount: f.chunkCount,
        });
      }
    }
    searchParams.delete("f");
    setSearchParams(searchParams, { replace: true });
  }, [fileParam, projectData, searchParams, setSearchParams]);

  const { data: detail, isFetching: detailLoading } = useQuery({
    queryKey: ["conversation", selectedConvId],
    queryFn: () => {
      if (!selectedConvId) throw new Error("no conversation selected");
      return apiFetch<ConversationDetail>(`/api/conversations/${selectedConvId}`);
    },
    enabled: selectedConvId !== null,
    retry: false,
  });

  useEffect(() => {
    if (selectedConvId === null) {
      seededRef.current = null;
      reset();
      return;
    }
    if (detail && seededRef.current !== selectedConvId) {
      seededRef.current = selectedConvId;
      setMessages(detail.messages.map(toChatMessage));
      if (detail.context) {
        setContextStatus(detail.context);
      }
      setSendError(null);
    }
  }, [selectedConvId, detail, reset, setMessages]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, trace, pending]);

  async function handleProjectFileUpload(files: FileList | null) {
    if (!files || files.length === 0) return;
    for (const f of Array.from(files)) {
      uploadFile.mutate(f, {
        onSuccess: () => toast.success(`Uploaded ${f.name}`),
        onError: (err) => toast.error(err instanceof Error ? err.message : `Failed to upload ${f.name}`),
      });
    }
    if (projectFileInputRef.current) projectFileInputRef.current.value = "";
  }

  async function pickComposerFiles(files: FileList | null) {
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

  async function submit() {
    const text = input.trim();
    const totalFiles = attachments.length + attachedFiles.length;
    if ((!text && totalFiles === 0) || status === "running" || pending.length > 0 || extractingFiles) return;
    setSendError(null);
    const images = attachments.map(({ mediaType, data }) => ({ mediaType, data }));
    const outgoingText = formatPromptWithFiles(text, attachedFiles);
    setInput("");
    setAttachments([]);
    setAttachedFiles([]);

    try {
      let convId = selectedConvId;
      if (convId === null) {
        const conv = await createProjectConv.mutateAsync({ title: text.slice(0, 30) || "Project Chat" });
        convId = conv.id;
        setSelectedConvId(convId);
        seededRef.current = convId;
        if (convParam) {
          searchParams.delete("c");
          setSearchParams(searchParams, { replace: true });
        }
      }
      await send(outgoingText, convId, images);
    } catch (err) {
      setSendError(err instanceof ApiError ? err.message : "Failed to send message");
    }
  }

  const voice = useVoiceInput({
    onInterim: (t) => setInput((p) => (p ? `${p} ${t}` : t)),
    onFinal: (t) => setInput((p) => (p ? `${p} ${t}` : t)),
  });

  function handleStartNewChat() {
    setSelectedConvId(null);
    seededRef.current = null;
    reset();
    setInput("");
    setSendError(null);
    searchParams.set("c", "new");
    setSearchParams(searchParams, { replace: true });
    setTimeout(() => textareaRef.current?.focus(), 50);
  }

  const project = projectData?.project;
  const projectFiles = projectData?.files ?? [];

  if (projectLoading) {
    return (
      <div className="flex h-full flex-col p-6 space-y-4">
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-full w-full rounded-2xl" />
      </div>
    );
  }

  if (projectError || !project) {
    return (
      <div className="flex h-full flex-col items-center justify-center p-6 space-y-3">
        <p className="text-sm text-destructive">Project not found.</p>
        <Button variant="outline" size="sm" asChild>
          <Link to="/projects">Back to Projects</Link>
        </Button>
      </div>
    );
  }

  const activeEstimatedTokens = contextStatus?.estimatedTokens ?? messages.reduce((acc, m) => acc + Math.round(m.content.length / 4) + 40, 0);
  const activeThresholdTokens = contextStatus?.thresholdTokens ?? 40000;
  const activeKeepRecent = contextStatus?.keepRecent ?? 10;
  const isCompacted = contextStatus?.compacted ?? false;

  return (
    <div className="flex h-full flex-col bg-background">
      {/* Workspace Header */}
      <header className="flex items-center justify-between gap-2 border-b border-border/60 px-4 py-2 bg-background/80 backdrop-blur-xs z-10 shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => {
              if (window.innerWidth < 768) setMobileOpen(true);
              else toggleSidebar();
            }}
            aria-label="Toggle sidebar"
            title="Toggle sidebar (⌘B)"
            className="text-muted-foreground hover:text-foreground shrink-0"
          >
            <Sidebar className="size-4" />
          </Button>

          <Button variant="ghost" size="icon-xs" asChild className="text-muted-foreground hover:text-foreground shrink-0">
            <Link to="/projects" title="Back to projects list">
              <ArrowLeft className="size-4" />
            </Link>
          </Button>

          <div className="flex items-center gap-1.5 min-w-0">
            <FolderGit2 className="size-4 text-primary shrink-0" />
            <h1 className="text-sm font-semibold tracking-tight truncate shrink-0">{project.name}</h1>

            <span className="text-muted-foreground/40 font-light shrink-0">/</span>

            {/* Conversation Switcher Dropdown */}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button
                  type="button"
                  className="flex items-center gap-1.5 max-w-[160px] sm:max-w-[240px] md:max-w-[320px] rounded-md px-2 py-1 text-xs text-muted-foreground hover:text-foreground hover:bg-muted/50 transition-colors"
                >
                  <MessageSquare className="size-3 text-muted-foreground shrink-0" />
                  <span className="truncate font-medium">
                    {selectedConvId
                      ? cleanConversationTitle(projectData?.conversations.find((c) => c.id === selectedConvId)?.title || "Chat")
                      : "New chat"}
                  </span>
                  <ChevronDown className="size-3 text-muted-foreground/60 shrink-0 ml-0.5" />
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="w-72 max-h-80 overflow-y-auto">
                <DropdownMenuItem
                  onClick={handleStartNewChat}
                  className="gap-2 text-emerald-600 dark:text-emerald-400 font-medium cursor-pointer"
                >
                  <Plus className="size-3.5" />
                  <span>New chat in this project</span>
                </DropdownMenuItem>

                {projectData && projectData.conversations.length > 0 && <DropdownMenuSeparator />}

                {(!projectData || projectData.conversations.length === 0) ? (
                  <div className="px-2 py-1.5 text-xs text-muted-foreground">No conversations yet</div>
                ) : (
                  projectData.conversations.map((c) => {
                    const isSelected = c.id === selectedConvId;
                    return (
                      <DropdownMenuItem
                        key={c.id}
                        onClick={() => {
                          setSelectedConvId(c.id);
                          searchParams.set("c", c.id);
                          setSearchParams(searchParams, { replace: true });
                        }}
                        className="flex items-center justify-between gap-2 cursor-pointer"
                      >
                        <div className="flex items-center gap-2 min-w-0 flex-1">
                          {isSelected ? (
                            <Check className="size-3.5 text-emerald-600 shrink-0" />
                          ) : (
                            <MessageSquare className="size-3.5 text-muted-foreground shrink-0" />
                          )}
                          <span className={cn("truncate text-xs", isSelected && "font-medium text-foreground")}>
                            {cleanConversationTitle(c.title)}
                          </span>
                        </div>
                      </DropdownMenuItem>
                    );
                  })
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>

        <div className="flex items-center gap-1.5 shrink-0">
          <Button
            variant={filesOpen ? "secondary" : "ghost"}
            size="sm"
            onClick={() => setFilesOpen((o) => !o)}
            className="text-xs gap-1.5 h-8 font-medium"
          >
            <FileText className="size-3.5" />
            <span>Files ({projectFiles.length})</span>
          </Button>

          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => setInstructionsOpen(true)}
            title="Project guidelines & instructions"
            className="text-muted-foreground hover:text-foreground"
          >
            <SlidersHorizontal className="size-3.5" />
          </Button>
        </div>
      </header>

      {/* Main split view: Chat (left) + Files Panel (right) */}
      <div className="flex min-h-0 flex-1 overflow-hidden">
        {/* Project Chat Area */}
        <div className="flex flex-1 flex-col overflow-hidden">
          <ScrollArea className="min-h-0 flex-1">
            <div className="mx-auto w-full max-w-3xl px-4 py-6">
              {messages.length === 0 && !detailLoading ? (
                <div className="flex min-h-[50vh] flex-col items-center justify-center gap-4 text-center">
                  <div className="flex size-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                    <FolderGit2 className="size-6" />
                  </div>
                  <div className="space-y-1">
                    <h2 className="text-xl font-semibold">{project.name} Workspace</h2>
                    <p className="max-w-md text-xs text-muted-foreground">
                      {project.description || "Chat grounded in your project documents. Herbie searches your project files first before checking the web."}
                    </p>
                  </div>
                  {projectFiles.length > 0 ? (
                    <div className="rounded-xl border border-border/60 bg-muted/20 px-3 py-2 text-xs text-muted-foreground flex items-center gap-2">
                      <CheckCircle2 className="size-3.5 text-primary" />
                      <span>{projectFiles.length} {projectFiles.length === 1 ? "file" : "files"} indexed and ready for retrieval</span>
                    </div>
                  ) : (
                    <Button size="sm" variant="outline" onClick={() => projectFileInputRef.current?.click()}>
                      <Upload className="size-3.5" /> Upload files to this project
                    </Button>
                  )}
                </div>
              ) : (
                <div className="space-y-5">
                  {messages.map((m, index) => {
                    const isLast = index === messages.length - 1;
                    const blockInteraction = status === "running" || pending.length > 0;
                    return (
                      <div key={m.id} className={cn("group flex", m.role === "user" ? "justify-end" : "gap-3")}>
                        {m.role === "user" ? (
                          <div className="group/user flex max-w-[80%] flex-col items-end gap-1">
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
                                          <span className="font-mono text-[11px] font-medium">{file.name}</span>
                                          <Eye className="size-3 text-muted-foreground opacity-60 group-hover/file:opacity-100 group-hover/file:text-primary transition-opacity" />
                                        </button>
                                      ))}
                                    </div>
                                  ) : filenames.length > 0 ? (
                                    <div className="flex flex-wrap justify-end gap-1.5 mb-1">
                                      {filenames.map((fname, idx) => (
                                        <div key={idx} className="flex items-center gap-1.5 rounded-xl border border-border/80 bg-muted/50 px-2.5 py-1 text-xs">
                                          <FileCode className="size-3.5 text-primary" />
                                          <span className="font-mono text-[11px]">{fname}</span>
                                        </div>
                                      ))}
                                    </div>
                                  ) : null}
                                  {(userPrompt || filenames.length > 0) && (
                                    <div className="rounded-2xl rounded-br-md bg-primary px-4 py-2.5 whitespace-pre-wrap text-primary-foreground text-sm shadow-sm">
                                      {userPrompt || (filenames.length === 1 ? `Attached ${filenames[0]}` : `Attached ${filenames.length} files`)}
                                    </div>
                                  )}
                                  <div className="flex items-center gap-1.5 opacity-0 transition-opacity group-hover/user:opacity-100 focus-within:opacity-100">
                                    <CopyMessageButton text={userPrompt || filenames.join(", ")} />
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
                                  </div>
                                </>
                              );
                            })()}
                          </div>
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
                                          // Citation click opens the project's indexed source.
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
                              {!m.streaming && !m.error && (
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
                        <Button size="sm" onClick={() => decide(true)} disabled={status === "running"}>
                          Approve
                        </Button>
                        <Button size="sm" variant="outline" onClick={() => decide(false)} disabled={status === "running"}>
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

          {/* Project Composer */}
          <div className="p-3 md:p-4 bg-gradient-to-t from-background via-background to-transparent shrink-0">
            <div className="mx-auto w-full max-w-3xl">
              {sendError && <p className="mb-2 text-destructive text-xs">{sendError}</p>}
              {attachedFiles.length > 0 && (
                <div className="mb-2 flex flex-wrap gap-2">
                  {attachedFiles.map((doc) => (
                    <div key={doc.id} className="flex items-center gap-1.5 rounded-xl border border-border/80 bg-muted/50 px-2.5 py-1 text-xs shadow-xs">
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
                        onClick={() => setAttachedFiles((p) => p.filter((f) => f.id !== doc.id))}
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

              <div className="rounded-2xl border border-border/80 bg-card/95 backdrop-blur-md shadow-md focus-within:ring-2 focus-within:ring-ring/40">
                <div className="p-2.5 pb-1">
                  <textarea
                    ref={textareaRef}
                    value={input}
                    onChange={(e) => setInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && !e.shiftKey) {
                        e.preventDefault();
                        void submit();
                      }
                    }}
                    rows={1}
                    placeholder="Ask about this project…"
                    className="max-h-48 min-h-9 w-full resize-none bg-transparent px-2 py-1 text-sm outline-none placeholder:text-muted-foreground/60 field-sizing-content"
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
                      onChange={(e) => void pickComposerFiles(e.target.files)}
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => fileInputRef.current?.click()}
                      disabled={status === "running" || pending.length > 0 || extractingFiles}
                      className="text-muted-foreground hover:text-foreground"
                      title="Attach file to message"
                    >
                      <Paperclip className="size-3.5" />
                    </Button>
                    <span className="text-[11px] text-muted-foreground font-medium px-1 flex items-center gap-1">
                      <FolderGit2 className="size-3 text-primary" />
                      <span>RAG first</span>
                    </span>
                    <ContextStatusMeter
                      estimatedTokens={activeEstimatedTokens}
                      thresholdTokens={activeThresholdTokens}
                      keepRecent={activeKeepRecent}
                      compacted={isCompacted}
                    />
                  </div>

                  <div className="flex items-center gap-1.5">
                    {voice.supported && (
                      <Button
                        size="icon-xs"
                        variant={voice.listening ? "default" : "ghost"}
                        onClick={voice.toggle}
                        className="text-muted-foreground hover:text-foreground"
                      >
                        {voice.listening ? <Square className="size-3" /> : <Mic className="size-3.5" />}
                      </Button>
                    )}
                    {status === "running" ? (
                      <Button size="icon-xs" onClick={stop} className="size-7">
                        <Square className="size-3 fill-current" />
                      </Button>
                    ) : (
                      <Button size="icon-xs" onClick={() => void submit()} className="size-7" disabled={!input.trim() && attachedFiles.length === 0}>
                        <ArrowUp className="size-3.5" />
                      </Button>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Separate File Management Panel */}
        {filesOpen && (
          <aside className="w-80 border-l border-border/60 bg-muted/10 flex flex-col shrink-0 overflow-hidden">
            <div className="flex items-center justify-between p-3 border-b border-border/60 bg-background/50">
              <div className="flex items-center gap-2">
                <FileText className="size-4 text-muted-foreground" />
                <h3 className="text-xs font-semibold uppercase tracking-wider">Project Files</h3>
                <Badge variant="secondary" className="text-[10px] px-1.5 py-0 font-mono">
                  {projectFiles.length}
                </Badge>
              </div>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => setFilesOpen(false)}
                className="text-muted-foreground hover:text-foreground"
              >
                <X className="size-3.5" />
              </Button>
            </div>

            {/* Drag & drop upload box */}
            <div className="p-3 border-b border-border/40">
              <input
                ref={projectFileInputRef}
                type="file"
                multiple
                accept=".txt,.md,.pdf,.docx,.xlsx,.pptx,.csv,.tsv,.json,.yaml,.yml,.py,.js,.ts,.tsx,.jsx,.go,.rs,.sh,.sql"
                hidden
                onChange={(e) => void handleProjectFileUpload(e.target.files)}
              />
              <button
                type="button"
                onClick={() => projectFileInputRef.current?.click()}
                onDragOver={(e) => {
                  e.preventDefault();
                  setProjectFileDragging(true);
                }}
                onDragLeave={() => setProjectFileDragging(false)}
                onDrop={(e) => {
                  e.preventDefault();
                  setProjectFileDragging(false);
                  void handleProjectFileUpload(e.dataTransfer.files);
                }}
                className={cn(
                  "w-full flex flex-col items-center justify-center gap-1.5 rounded-xl border border-dashed p-4 text-center transition-colors",
                  projectFileDragging
                    ? "border-primary bg-primary/5"
                    : "border-border/80 hover:bg-muted/40"
                )}
              >
                <Upload className="size-4 text-muted-foreground" />
                <span className="text-xs font-medium">Upload project file</span>
                <span className="text-[10px] text-muted-foreground">PDF, DOCX, XLSX, PPTX, TXT, MD, CSV</span>
              </button>
            </div>

            {/* File List */}
            <ScrollArea className="flex-1 p-2">
              <div className="space-y-1.5">
                {projectFiles.length === 0 ? (
                  <p className="py-6 text-center text-xs text-muted-foreground">
                    No files uploaded to this project yet.
                  </p>
                ) : (
                  projectFiles.map((f) => (
                    <div
                      key={f.id}
                      onClick={() =>
                        setViewingFile({
                          title: f.filename,
                          documentId: f.id,
                          sizeBytes: f.sizeBytes,
                          status: f.status,
                          chunkCount: f.chunkCount,
                        })
                      }
                      className="group flex items-center justify-between gap-2 rounded-xl border border-border/50 bg-card p-2.5 text-xs shadow-2xs hover:border-border transition-colors cursor-pointer"
                    >
                      <div className="min-w-0 flex-1">
                        <p className="font-medium truncate text-foreground group-hover:text-primary transition-colors" title={f.filename}>
                          {f.filename}
                        </p>
                        <div className="flex items-center gap-2 mt-0.5 text-[10px] text-muted-foreground">
                          <span>{Math.ceil(f.sizeBytes / 1024)} KB</span>
                          {f.chunkCount > 0 && <span>· {f.chunkCount} chunks</span>}
                          <span
                            className={cn(
                              "rounded-full px-1.5 py-0.2 font-medium",
                              f.status === "ready"
                                ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                                : "bg-amber-500/10 text-amber-600"
                            )}
                          >
                            {f.status}
                          </span>
                        </div>
                      </div>
                      <div className="flex items-center gap-0.5" onClick={(e) => e.stopPropagation()}>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() =>
                            setViewingFile({
                              title: f.filename,
                              documentId: f.id,
                              sizeBytes: f.sizeBytes,
                              status: f.status,
                              chunkCount: f.chunkCount,
                            })
                          }
                          className="text-muted-foreground hover:text-foreground"
                          aria-label={`View parsed ${f.filename}`}
                          title={`View parsed content of ${f.filename}`}
                        >
                          <Eye className="size-3" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={async () => {
                            if (confirm(`Delete ${f.filename}?`)) {
                              await apiFetch(`/api/documents/${f.id}`, { method: "DELETE" });
                              void queryClient.invalidateQueries({ queryKey: ["project", projectId] });
                              toast.success("File deleted");
                            }
                          }}
                          className="opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-destructive transition-opacity"
                          aria-label={`Delete ${f.filename}`}
                        >
                          <Trash2 className="size-3" />
                        </Button>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </ScrollArea>
          </aside>
        )}
      </div>

      {/* Instructions Dialog */}
      <Dialog open={instructionsOpen} onOpenChange={setInstructionsOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Project Guidelines</DialogTitle>
            <DialogDescription>
              Instructions applied to every conversation in this project.
            </DialogDescription>
          </DialogHeader>
          <div className="py-3">
            <p className="text-xs text-foreground bg-muted/40 rounded-xl p-3 whitespace-pre-wrap">
              {project.instructions || "No custom instructions set for this project."}
            </p>
          </div>
          <DialogFooter>
            <Button size="sm" onClick={() => setInstructionsOpen(false)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Message Confirmation Dialog */}
      <Dialog
        open={pendingMessageDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingMessageDelete(null);
        }}
      >
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete message?</DialogTitle>
            <DialogDescription>
              This message and every message after it will be permanently removed from this conversation.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="ghost" onClick={() => setPendingMessageDelete(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={confirmDeleteMessage} disabled={deleteMessage.isPending}>
              {deleteMessage.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

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
