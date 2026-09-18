import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  Check,
  Code2,
  Copy,
  FileText,
  Maximize2,
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
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";

interface PayloadViewerProps {
  data: unknown;
  title?: string;
  defaultMode?: "rich" | "raw";
  className?: string;
}

// Known text/markdown property keys commonly returned by AI, API, or Transform nodes
const TEXT_KEYS = ["analysis", "text", "content", "summary", "prompt", "message", "body", "output", "notes", "result", "markdown"];

export function PayloadViewer({
  data,
  title = "Payload",
  defaultMode = "rich",
  className,
}: PayloadViewerProps) {
  const [mode, setMode] = useState<"rich" | "raw">(defaultMode);
  const [copied, setCopied] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);

  function copyPayload() {
    const text = typeof data === "string" ? data : JSON.stringify(data, null, 2);
    navigator.clipboard.writeText(text);
    setCopied(true);
    toast.success("Payload copied to clipboard");
    setTimeout(() => setCopied(false), 1500);
  }

  // Determine if data contains rich text/markdown
  const hasRichText = typeof data === "string" || isObjectWithText(data);

  return (
    <div className={cn("flex flex-col rounded-xl border border-border bg-card shadow-2xs overflow-hidden", className)}>
      {/* Header bar */}
      <div className="flex items-center justify-between border-b border-border/70 bg-muted/30 px-3 py-2 text-xs">
        <div className="flex items-center gap-2 min-w-0">
          <span className="font-semibold text-foreground text-xs truncate">{title}</span>
          {typeof data === "object" && data !== null && (
            <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono h-4 shrink-0">
              {Array.isArray(data) ? `${data.length} items` : `${Object.keys(data).length} fields`}
            </Badge>
          )}
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {/* View mode toggle */}
          {hasRichText && (
            <div className="flex rounded-md border border-border bg-background p-0.5 mr-1">
              <button
                type="button"
                onClick={() => setMode("rich")}
                className={cn(
                  "flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium transition-colors",
                  mode === "rich"
                    ? "bg-primary text-primary-foreground font-semibold"
                    : "text-muted-foreground hover:text-foreground"
                )}
                title="Rich text / Markdown preview"
              >
                <FileText className="size-3" />
                <span>Rich</span>
              </button>
              <button
                type="button"
                onClick={() => setMode("raw")}
                className={cn(
                  "flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium transition-colors",
                  mode === "raw"
                    ? "bg-primary text-primary-foreground font-semibold"
                    : "text-muted-foreground hover:text-foreground"
                )}
                title="Raw JSON code view"
              >
                <Code2 className="size-3" />
                <span>JSON</span>
              </button>
            </div>
          )}

          <Button
            variant="ghost"
            size="icon-xs"
            onClick={copyPayload}
            className="size-6 text-muted-foreground hover:text-foreground"
            title="Copy payload"
          >
            {copied ? <Check className="size-3 text-emerald-500" /> : <Copy className="size-3" />}
          </Button>

          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => setFullscreen(true)}
            className="size-6 text-muted-foreground hover:text-foreground"
            title="Expand to fullscreen reader"
          >
            <Maximize2 className="size-3" />
          </Button>
        </div>
      </div>

      {/* Main Content Viewer */}
      <div className="p-3">
        {mode === "rich" && hasRichText ? (
          <RichPayloadRenderer data={data} />
        ) : (
          <RawJSONRenderer data={data} />
        )}
      </div>

      {/* Expanded Fullscreen Dialog */}
      <Dialog open={fullscreen} onOpenChange={setFullscreen}>
        <DialogContent className="max-w-4xl max-h-[88vh] flex flex-col p-0 overflow-hidden">
          <DialogHeader className="flex flex-row items-center justify-between border-b border-border p-4 bg-muted/20">
            <DialogTitle className="text-sm font-semibold tracking-tight">{title}</DialogTitle>
            <div className="flex items-center gap-2 pr-6">
              {hasRichText && (
                <div className="flex rounded-md border border-border bg-background p-0.5">
                  <button
                    type="button"
                    onClick={() => setMode("rich")}
                    className={cn(
                      "flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors",
                      mode === "rich" ? "bg-primary text-primary-foreground font-semibold" : "text-muted-foreground"
                    )}
                  >
                    <FileText className="size-3" />
                    <span>Rich Text</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => setMode("raw")}
                    className={cn(
                      "flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors",
                      mode === "raw" ? "bg-primary text-primary-foreground font-semibold" : "text-muted-foreground"
                    )}
                  >
                    <Code2 className="size-3" />
                    <span>Raw JSON</span>
                  </button>
                </div>
              )}
              <Button variant="outline" size="sm" onClick={copyPayload} className="h-7 text-xs gap-1.5">
                {copied ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
                <span>Copy</span>
              </Button>
            </div>
          </DialogHeader>

          <ScrollArea className="flex-1 p-6">
            {mode === "rich" && hasRichText ? (
              <div className="max-w-3xl mx-auto">
                <RichPayloadRenderer data={data} expanded />
              </div>
            ) : (
              <RawJSONRenderer data={data} expanded />
            )}
          </ScrollArea>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function isObjectWithText(data: unknown): boolean {
  if (typeof data !== "object" || data === null) return false;
  const obj = data as Record<string, unknown>;
  return Object.keys(obj).some((k) => TEXT_KEYS.includes(k.toLowerCase()) && typeof obj[k] === "string");
}

function RichPayloadRenderer({ data, expanded }: { data: unknown; expanded?: boolean }) {
  // If data is directly a string
  if (typeof data === "string") {
    return (
      <div className={cn("prose prose-xs dark:prose-invert max-w-none text-xs leading-relaxed break-words", expanded && "prose-sm")}>
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{data}</ReactMarkdown>
      </div>
    );
  }

  // If data is an object with one or more text/markdown fields
  if (typeof data === "object" && data !== null) {
    const obj = data as Record<string, unknown>;
    const textFields: Array<{ key: string; value: string }> = [];
    const metaFields: Array<{ key: string; value: unknown }> = [];

    for (const [k, val] of Object.entries(obj)) {
      if (typeof val === "string" && (TEXT_KEYS.includes(k.toLowerCase()) || val.length > 60 || val.includes("\n"))) {
        textFields.push({ key: k, value: val });
      } else {
        metaFields.push({ key: k, value: val });
      }
    }

    return (
      <div className="space-y-3.5">
        {/* Render text/markdown content sections */}
        {textFields.map(({ key, value }) => (
          <div key={key} className="rounded-lg border border-border/80 bg-muted/20 p-3.5 space-y-2">
            <div className="flex items-center justify-between border-b border-border/50 pb-1.5">
              <span className="text-[11px] font-semibold tracking-wider text-muted-foreground uppercase font-mono">
                {key}
              </span>
              <Badge variant="outline" className="text-[9px] font-mono">
                markdown
              </Badge>
            </div>
            <div className="text-xs leading-relaxed break-words space-y-2 text-foreground font-normal [&_h1]:text-sm [&_h1]:font-bold [&_h2]:text-xs [&_h2]:font-bold [&_h3]:text-xs [&_h3]:font-semibold [&_p]:my-1.5 [&_ul]:list-disc [&_ul]:pl-4 [&_ol]:list-decimal [&_ol]:pl-4 [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[11px] [&_pre]:rounded [&_pre]:bg-muted/80 [&_pre]:p-2 [&_pre]:font-mono [&_pre]:text-[11px] [&_blockquote]:border-l-2 [&_blockquote]:border-primary/40 [&_blockquote]:pl-2.5 [&_blockquote]:italic">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{value}</ReactMarkdown>
            </div>
          </div>
        ))}

        {/* Render metadata properties as clean structured badges/table */}
        {metaFields.length > 0 && (
          <div className="rounded-lg border border-border/60 bg-muted/10 p-2.5 space-y-1.5">
            <p className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider font-mono">
              Fields & Parameters
            </p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs">
              {metaFields.map(({ key, value }) => (
                <div
                  key={key}
                  className="flex items-center justify-between rounded border border-border/40 bg-background/50 px-2.5 py-1.5"
                >
                  <span className="text-muted-foreground font-mono text-[11px] truncate">{key}:</span>
                  <span className="font-semibold text-foreground font-mono text-[11px] truncate ml-2">
                    {formatMetaValue(value)}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    );
  }

  return <RawJSONRenderer data={data} expanded={expanded} />;
}

function formatMetaValue(val: unknown): string {
  if (val === null) return "null";
  if (val === undefined) return "undefined";
  if (typeof val === "object") return JSON.stringify(val);
  return String(val);
}

function RawJSONRenderer({ data, expanded }: { data: unknown; expanded?: boolean }) {
  const jsonStr = JSON.stringify(data ?? null, null, 2);

  return (
    <pre
      className={cn(
        "w-full overflow-x-auto rounded-lg border border-border bg-muted/40 p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words text-foreground",
        !expanded && "max-h-72"
      )}
    >
      {jsonStr}
    </pre>
  );
}
