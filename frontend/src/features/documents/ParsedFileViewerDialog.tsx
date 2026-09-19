import { useMemo, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import rehypeKatex from "rehype-katex";
import { useQuery } from "@tanstack/react-query";
import {
  Check,
  Code2,
  Copy,
  Download,
  FileCode,
  FileText,
  Loader2,
  Search,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { CodeBlock } from "@/components/ai/CodeBlock";
import { apiFetch } from "@/lib/api";
import { cn } from "@/lib/utils";

export interface ParsedDocumentContent {
  id: string;
  filename: string;
  mime: string;
  sizeBytes: number;
  status: string;
  error?: string;
  chunkCount: number;
  createdAt: string;
  text: string;
}

export interface ParsedFileViewerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  documentId?: string | null;
  content?: string | null;
  sizeBytes?: number;
  status?: string;
  chunkCount?: number;
  mime?: string;
}

export function ParsedFileViewerDialog({
  open,
  onOpenChange,
  title,
  documentId,
  content,
  sizeBytes: initialSizeBytes,
  status: initialStatus,
  chunkCount: initialChunkCount,
}: ParsedFileViewerDialogProps) {
  const [copied, setCopied] = useState(false);
  const [viewMode, setViewMode] = useState<"rendered" | "raw">("rendered");
  const [searchQuery, setSearchQuery] = useState("");

  const shouldFetch = Boolean(documentId && content === undefined);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["document-content", documentId],
    queryFn: () => apiFetch<ParsedDocumentContent>(`/api/documents/${documentId}/content`),
    enabled: open && shouldFetch && Boolean(documentId),
    staleTime: 30_000,
  });

  const rawText = content ?? data?.text ?? "";
  const effectiveStatus = initialStatus ?? data?.status ?? "ready";
  const effectiveSize = initialSizeBytes ?? data?.sizeBytes ?? 0;
  const effectiveChunks = initialChunkCount ?? data?.chunkCount ?? 0;

  const lines = useMemo(() => {
    const l = rawText.split(/\r\n|\r|\n/);
    return l;
  }, [rawText]);
  const estimatedTokens = useMemo(() => Math.round(rawText.length / 4), [rawText]);

  // Search match statistics
  const searchMatches = useMemo(() => {
    if (!searchQuery.trim() || !rawText) return 0;
    const escaped = searchQuery.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const re = new RegExp(escaped, "gi");
    return (rawText.match(re) || []).length;
  }, [searchQuery, rawText]);

  function copyText() {
    if (!rawText) return;
    navigator.clipboard.writeText(rawText);
    setCopied(true);
    toast.success("Parsed text copied to clipboard");
    setTimeout(() => setCopied(false), 1600);
  }

  function downloadText() {
    if (!rawText) return;
    const baseName = title.replace(/\.[^/.]+$/, "");
    const blob = new Blob([rawText], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${baseName}-parsed.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    toast.success("Downloaded parsed file");
  }

  const isCodeOrData = /\.(py|js|jsx|ts|tsx|go|rs|sql|sh|json|yaml|yml|xml|csv|tsv|html|css)$/i.test(title);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[88vh] max-h-[88vh] flex-col gap-0 p-0 overflow-hidden sm:max-w-3xl lg:max-w-4xl">
        {/* Header */}
        <DialogHeader className="border-b border-border/70 px-5 py-3.5 bg-muted/20 shrink-0">
          <div className="flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between min-w-0 pr-6">
            <div className="flex items-center gap-2.5 min-w-0">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-xl border border-border/80 bg-background text-primary shadow-2xs">
                {isCodeOrData ? <FileCode className="size-4" /> : <FileText className="size-4" />}
              </span>
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <DialogTitle className="text-sm font-semibold truncate max-w-[280px] sm:max-w-[420px]" title={title}>
                    {title}
                  </DialogTitle>
                  <Badge
                    variant="outline"
                    className={cn(
                      "px-1.5 py-0 text-[10px] font-medium shrink-0",
                      effectiveStatus === "ready" && "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
                      effectiveStatus === "processing" && "border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400",
                      effectiveStatus === "failed" && "border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400"
                    )}
                  >
                    {effectiveStatus}
                  </Badge>
                </div>
                <div className="flex items-center gap-2 mt-0.5 text-[11px] text-muted-foreground font-mono">
                  {effectiveSize > 0 && <span>{Math.ceil(effectiveSize / 1024)} KB</span>}
                  {effectiveChunks > 0 && <span>· {effectiveChunks} chunks</span>}
                  {rawText.length > 0 && (
                    <>
                      <span>· {lines.length.toLocaleString()} lines</span>
                      <span>· ~{estimatedTokens.toLocaleString()} tokens</span>
                    </>
                  )}
                </div>
              </div>
            </div>

            {/* Actions */}
            <div className="flex items-center gap-1.5 shrink-0">
              <div className="flex rounded-lg border border-border/80 bg-background p-0.5 shadow-2xs">
                <button
                  type="button"
                  onClick={() => setViewMode("rendered")}
                  className={cn(
                    "flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition-colors",
                    viewMode === "rendered"
                      ? "bg-primary text-primary-foreground font-semibold shadow-2xs"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <FileText className="size-3" />
                  <span>Formatted</span>
                </button>
                <button
                  type="button"
                  data-testid="raw-mode-btn"
                  onClick={() => setViewMode("raw")}
                  className={cn(
                    "flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition-colors",
                    viewMode === "raw"
                      ? "bg-primary text-primary-foreground font-semibold shadow-2xs"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <Code2 className="size-3" />
                  <span>Raw Text</span>
                </button>
              </div>

              <Button
                variant="outline"
                size="sm"
                onClick={copyText}
                disabled={!rawText}
                className="h-7 text-xs gap-1 shadow-2xs"
                title="Copy parsed text"
              >
                {copied ? <Check className="size-3 text-emerald-500" /> : <Copy className="size-3" />}
                <span className="hidden sm:inline">{copied ? "Copied" : "Copy"}</span>
              </Button>

              <Button
                variant="outline"
                size="sm"
                onClick={downloadText}
                disabled={!rawText}
                className="h-7 px-2 text-xs shadow-2xs"
                title="Download parsed text"
              >
                <Download className="size-3" />
              </Button>
            </div>
          </div>

          {/* Search bar inside header toolbar */}
          {rawText.length > 0 && (
            <div className="relative mt-2.5">
              <Search className="absolute left-2.5 top-2 size-3.5 text-muted-foreground" />
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search within parsed file…"
                className="h-7.5 pl-8 pr-16 text-xs bg-background/80"
              />
              {searchQuery && (
                <div className="absolute right-2 top-1.5 flex items-center gap-1 text-[10px] text-muted-foreground">
                  <span>{searchMatches} {searchMatches === 1 ? "match" : "matches"}</span>
                  <button
                    type="button"
                    onClick={() => setSearchQuery("")}
                    className="p-0.5 hover:text-foreground rounded"
                    aria-label="Clear search"
                  >
                    <X className="size-3" />
                  </button>
                </div>
              )}
            </div>
          )}
        </DialogHeader>

        {/* Body Content */}
        <div className="flex-1 min-h-0 overflow-y-auto p-5 bg-card/60">
          {isLoading && (
            <div className="flex flex-col items-center justify-center h-64 gap-2.5 text-muted-foreground">
              <Loader2 className="size-6 animate-spin text-primary" />
              <p className="text-xs font-medium">Extracting and loading parsed content…</p>
            </div>
          )}

          {isError && (
            <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-center">
              <p className="text-xs font-medium text-destructive">
                {error instanceof Error ? error.message : "Failed to load document content."}
              </p>
            </div>
          )}

          {!isLoading && !isError && effectiveStatus === "processing" && !rawText && (
            <div className="flex flex-col items-center justify-center h-64 gap-2 text-muted-foreground text-center px-4">
              <Loader2 className="size-6 animate-spin text-amber-500" />
              <p className="text-sm font-semibold text-foreground">File is currently being processed</p>
              <p className="text-xs max-w-sm">
                Herbie is ingesting and chunking this document. Parsed text will become viewable once complete.
              </p>
            </div>
          )}

          {!isLoading && !isError && effectiveStatus === "failed" && !rawText && (
            <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-5 text-center">
              <p className="text-sm font-semibold text-destructive">Ingestion Failed</p>
              <p className="text-xs text-muted-foreground mt-1">
                {data?.error || "The server could not extract readable text from this file."}
              </p>
            </div>
          )}

          {!isLoading && !isError && rawText && (
            <>
              {viewMode === "rendered" ? (
                <div className="prose prose-sm dark:prose-invert max-w-none break-words leading-relaxed text-foreground">
                  <ReactMarkdown
                    remarkPlugins={[remarkGfm, remarkMath]}
                    rehypePlugins={[[rehypeKatex, { throwOnError: false }]]}
                    components={{
                      pre: ({ children }) => <CodeBlock>{children}</CodeBlock>,
                      h1: ({ children }) => (
                        <h1 className="text-xl font-bold mt-5 mb-2.5 pb-1.5 border-b border-border text-foreground first:mt-0">
                          {children}
                        </h1>
                      ),
                      h2: ({ children }) => (
                        <h2 className="text-lg font-semibold mt-4 mb-2 pb-1 border-b border-border/60 text-foreground first:mt-0">
                          {children}
                        </h2>
                      ),
                      h3: ({ children }) => (
                        <h3 className="text-base font-semibold mt-3 mb-1.5 text-foreground first:mt-0">
                          {children}
                        </h3>
                      ),
                      h4: ({ children }) => (
                        <h4 className="text-sm font-semibold mt-2 mb-1 text-foreground">
                          {children}
                        </h4>
                      ),
                      p: ({ children }) => (
                        <p className="my-2.5 leading-relaxed text-foreground/90 text-sm">
                          {children}
                        </p>
                      ),
                      ul: ({ children }) => (
                        <ul className="list-disc pl-5 my-2.5 space-y-1 text-sm text-foreground/90">
                          {children}
                        </ul>
                      ),
                      ol: ({ children }) => (
                        <ol className="list-decimal pl-5 my-2.5 space-y-1 text-sm text-foreground/90">
                          {children}
                        </ol>
                      ),
                      li: ({ children }) => <li className="leading-relaxed">{children}</li>,
                      blockquote: ({ children }) => (
                        <blockquote className="border-l-3 border-primary/50 bg-muted/30 px-3.5 py-1.5 my-3 italic text-muted-foreground rounded-r-md text-sm">
                          {children}
                        </blockquote>
                      ),
                      table: ({ children }) => (
                        <div className="my-3 w-full overflow-x-auto rounded-lg border border-border shadow-2xs">
                          <table className="w-full border-collapse text-xs text-left">
                            {children}
                          </table>
                        </div>
                      ),
                      thead: ({ children }) => (
                        <thead className="bg-muted/70 text-foreground font-semibold border-b border-border">
                          {children}
                        </thead>
                      ),
                      th: ({ children }) => (
                        <th className="px-3 py-2 font-semibold border-r border-border last:border-r-0">
                          {children}
                        </th>
                      ),
                      td: ({ children }) => (
                        <td className="px-3 py-1.5 border-t border-border/60 border-r border-border/60 last:border-r-0 text-foreground/90">
                          {children}
                        </td>
                      ),
                      hr: () => <hr className="my-5 border-border/80" />,
                    }}
                  >
                    {rawText}
                  </ReactMarkdown>
                </div>
              ) : (
                <div className="rounded-xl border border-border/80 bg-muted/30 font-mono text-xs overflow-x-auto shadow-2xs">
                  <table className="w-full border-collapse">
                    <tbody>
                      {lines.map((line, idx) => {
                        const lineNum = idx + 1;
                        const isMatch =
                          searchQuery.trim().length > 0 &&
                          line.toLowerCase().includes(searchQuery.toLowerCase());
                        return (
                          <tr
                            key={lineNum}
                            className={cn(
                              "hover:bg-muted/60 transition-colors",
                              isMatch && "bg-amber-500/15 dark:bg-amber-500/20 font-medium"
                            )}
                          >
                            <td className="w-12 py-0.5 pr-3 pl-3 text-right select-none text-[11px] text-muted-foreground/60 border-r border-border/50">
                              {lineNum}
                            </td>
                            <td className="py-0.5 px-3 whitespace-pre-wrap break-all text-foreground leading-relaxed">
                              {line || "\u00A0"}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
