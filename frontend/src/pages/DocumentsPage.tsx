import { useRef, useState } from "react";
import {
  CircleAlert,
  FileText,
  FileUp,
  Loader2,
  Sidebar,
  Trash2,
} from "lucide-react";
import { useSidebar } from "@/components/layout/SidebarContext";
import { cn } from "@/lib/utils";
import type { DocumentRec } from "@/lib/types";
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
import { Skeleton } from "@/components/ui/skeleton";
import { useDocumentActions, useDocuments } from "@/features/documents/useDocuments";
import { ApiError } from "@/lib/api";

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

const STATUS_STYLES: Record<DocumentRec["status"], string> = {
  processing: "border-amber-600/25 bg-amber-600/10 text-amber-700 dark:text-amber-400",
  ready: "border-emerald-600/25 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400",
  failed: "border-rose-600/25 bg-rose-600/10 text-rose-700 dark:text-rose-400",
};

export function DocumentsPage() {
  const { toggleSidebar, setMobileOpen } = useSidebar();
  const { data: docs, isLoading, isError, error, refetch, isFetching } = useDocuments();
  const { upload, remove, progress, accepting } = useDocumentActions();
  const [dragging, setDragging] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<DocumentRec | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const ragDisabled = error instanceof ApiError && error.code === "rag_disabled";

  const pick = (files: FileList | null) => {
    if (!files) return;
    for (const f of files) void upload(f);
  };

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
            <FileText className="size-4 text-muted-foreground" />
            <h1 className="text-sm font-semibold tracking-tight">Documents</h1>
            {docs && (
              <Badge variant="secondary" className="font-mono text-xs px-1.5 py-0 h-4">
                {docs.length}
              </Badge>
            )}
          </div>
        </div>
      </header>

      <div className="mx-auto w-full max-w-3xl space-y-4 overflow-y-auto p-4">
        <div
          role="button"
          tabIndex={0}
          aria-label="Upload documents"
          onClick={() => inputRef.current?.click()}
          onKeyDown={(e) => e.key === "Enter" && inputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragging(false);
            pick(e.dataTransfer.files);
          }}
          className={cn(
            "flex cursor-pointer flex-col items-center gap-2 rounded-xl border-2 border-dashed p-8 text-center outline-none transition-colors",
            dragging ? "border-primary bg-primary/5" : "border-border hover:border-primary/50 hover:bg-muted/40",
          )}
        >
          <FileUp className="text-muted-foreground" />
          <p className="text-sm font-medium">Drop files here or click to upload</p>
          <p className="text-muted-foreground text-xs">
            pdf, docx, xlsx, pptx, txt, md, csv, tsv, json — up to 20 MB. Documents are chunked, embedded, and searchable
            from chat.
          </p>
          <input
            ref={inputRef}
            type="file"
            accept={accepting}
            multiple
            hidden
            onChange={(e) => {
              pick(e.target.files);
              e.target.value = "";
            }}
          />
        </div>

        {progress !== null && (
          <div className="space-y-1">
            <div className="h-2 overflow-hidden rounded-full bg-muted">
              <div className="h-full rounded-full bg-primary transition-all" style={{ width: `${progress}%` }} />
            </div>
            <p className="text-muted-foreground text-right text-xs">Uploading… {progress}%</p>
          </div>
        )}

        {isLoading && (
          <div className="space-y-2">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-14 w-full" />
            ))}
          </div>
        )}

        {isError && (
          <div className="rounded-xl border border-dashed p-8 text-center">
            <p className="text-sm font-medium">
              {ragDisabled ? "Document search is disabled on this server" : "Couldn't load documents"}
            </p>
            <p className="mx-auto mt-1 max-w-sm text-muted-foreground text-xs">
              {ragDisabled
                ? "The backend runs without RAG_ENABLED and the rag compose profile. Start the stack with RAG on, then retry."
                : (error instanceof ApiError ? error.message : "Something went wrong.")}
            </p>
            <Button variant="outline" size="sm" className="mt-3" onClick={() => void refetch()} disabled={isFetching}>
              Retry
            </Button>
          </div>
        )}

        {docs && docs.length === 0 && (
          <p className="py-8 text-center text-muted-foreground text-sm">
            No documents yet. Upload one and ask the chat about it.
          </p>
        )}

        {docs && docs.length > 0 && (
          <ul className="divide-y rounded-xl border">
            {docs.map((d) => (
              <li key={d.id} className="flex items-center gap-3 p-3">
                <FileText className="shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium" title={d.filename}>
                    {d.filename}
                  </p>
                  <p className="text-muted-foreground text-xs">
                    {formatBytes(d.sizeBytes)} · {new Date(d.createdAt).toLocaleDateString()}
                    {d.status === "ready" && ` · ${d.chunkCount} chunks`}
                  </p>
                  {d.status === "failed" && d.error && (
                    <p className="mt-0.5 flex items-center gap-1 text-destructive text-xs">
                      <CircleAlert className="size-3" /> {d.error}
                    </p>
                  )}
                </div>
                <Badge variant="outline" className={cn("gap-1", STATUS_STYLES[d.status])}>
                  {d.status === "processing" && <Loader2 className="size-3 animate-spin" />}
                  {d.status}
                </Badge>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Delete ${d.filename}`}
                  className="text-destructive hover:text-destructive"
                  onClick={() => setPendingDelete(d)}
                >
                  <Trash2 />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>

      <Dialog open={pendingDelete !== null} onOpenChange={(open) => !open && setPendingDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete document?</DialogTitle>
            <DialogDescription>
              {pendingDelete?.filename} and its searchable chunks will be removed. This cannot be
              undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (pendingDelete) remove.mutate(pendingDelete.id);
                setPendingDelete(null);
              }}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
