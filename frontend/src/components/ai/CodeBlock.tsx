import React, { isValidElement, useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy, Code as CodeIcon, Eye, Maximize2, RotateCcw, Loader2 } from "lucide-react";
import { useTheme } from "next-themes";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function extractText(node: React.ReactNode): string {
  if (typeof node === "string") return node;
  if (typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(extractText).join("");
  if (isValidElement(node)) {
    const props = node.props as { children?: React.ReactNode };
    return extractText(props.children);
  }
  return "";
}

function getCodeDetails(children: React.ReactNode): { language: string; code: string } {
  if (isValidElement(children)) {
    const props = children.props as { className?: string; children?: React.ReactNode };
    const match = /language-([a-zA-Z0-9_-]+)/.exec(props.className || "");
    const language = match ? match[1].toLowerCase() : "";
    const code = extractText(props.children);
    return { language, code };
  }
  return { language: "", code: extractText(children) };
}

let mermaidPromise: Promise<typeof import("mermaid")> | null = null;
function getMermaid() {
  if (!mermaidPromise) {
    mermaidPromise = import("mermaid").then((m) => {
      m.default.initialize({
        startOnLoad: false,
        securityLevel: "loose",
        fontFamily: "inherit",
      });
      return m;
    });
  }
  return mermaidPromise;
}

export function MermaidPreview({
  code,
  isDark,
  streaming,
}: {
  code: string;
  isDark: boolean;
  streaming?: boolean;
}) {
  const [svg, setSvg] = useState<string>("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);

    const trimmed = code.trim();
    if (!trimmed) {
      setLoading(false);
      return;
    }

    getMermaid()
      .then(async ({ default: mermaid }) => {
        try {
          mermaid.initialize({
            startOnLoad: false,
            theme: isDark ? "dark" : "default",
            securityLevel: "loose",
          });
          const id = `mermaid-${Math.random().toString(36).slice(2, 9)}`;
          const res = await mermaid.render(id, trimmed);
          if (active) {
            setSvg(res.svg);
            setError(null);
            setLoading(false);
          }
        } catch (err) {
          if (active) {
            setError(err instanceof Error ? err.message : "Syntax error in diagram");
            setLoading(false);
          }
        }
      })
      .catch(() => {
        if (active) {
          setError("Failed to load mermaid engine");
          setLoading(false);
        }
      });

    return () => {
      active = false;
    };
  }, [code, isDark]);

  if (loading && !svg) {
    return (
      <div className="flex items-center justify-center p-8 text-xs text-muted-foreground" data-testid="mermaid-loading">
        <Loader2 className="mr-2 size-4 animate-spin" />
        Rendering diagram...
      </div>
    );
  }

  if (error) {
    if (streaming) {
      return (
        <div className="flex items-center justify-center p-8 text-xs text-muted-foreground" data-testid="mermaid-streaming">
          <Loader2 className="mr-2 size-4 animate-spin" />
          Rendering diagram...
        </div>
      );
    }
    return (
      <div className="p-4 text-xs bg-destructive/10 text-destructive rounded-md space-y-1" data-testid="mermaid-error">
        <p className="font-semibold">Unable to render diagram</p>
        <p className="font-mono text-[11px] opacity-80">{error}</p>
      </div>
    );
  }

  return (
    <div
      className="p-4 flex justify-center items-center overflow-x-auto bg-background/50 rounded-md"
      data-testid="mermaid-svg"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}

export function SvgPreview({ code }: { code: string }) {
  const trimmed = code.trim();
  const isSvg = /<svg[\s\S]*<\/svg>/i.test(trimmed);
  if (!isSvg) {
    return (
      <div className="p-4 text-xs text-muted-foreground" data-testid="svg-error">
        Incomplete or invalid SVG markup.
      </div>
    );
  }
  return (
    <div className="flex items-center justify-center p-6 bg-muted/20 rounded-md border overflow-auto" data-testid="svg-preview">
      <div
        className="max-w-full max-h-[400px] flex items-center justify-center [&_svg]:max-w-full [&_svg]:h-auto"
        dangerouslySetInnerHTML={{ __html: trimmed }}
      />
    </div>
  );
}

export function HtmlPreview({ code, isDark }: { code: string; isDark: boolean }) {
  const [reloadKey, setReloadKey] = useState(0);
  const doc = useMemo(() => {
    const trimmed = code.trim();
    if (/<!doctype\s+html/i.test(trimmed) || /<html/i.test(trimmed)) {
      return trimmed;
    }
    return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    :root {
      color-scheme: ${isDark ? "dark" : "light"};
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    }
    body {
      margin: 16px;
      background: ${isDark ? "#121212" : "#ffffff"};
      color: ${isDark ? "#f3f4f6" : "#111827"};
      line-height: 1.5;
    }
  </style>
</head>
<body>
${trimmed}
</body>
</html>`;
  }, [code, isDark]);

  return (
    <div className="relative flex flex-col rounded-md border bg-background overflow-hidden" data-testid="html-preview">
      <div className="flex items-center justify-between border-b bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
        <span className="font-mono text-[11px]">Sandboxed Preview</span>
        <button
          type="button"
          aria-label="Reload preview"
          title="Reload preview"
          onClick={() => setReloadKey((k) => k + 1)}
          className="rounded p-1 hover:bg-accent hover:text-foreground transition-colors cursor-pointer"
        >
          <RotateCcw className="size-3.5" />
        </button>
      </div>
      <iframe
        key={reloadKey}
        srcDoc={doc}
        sandbox="allow-scripts"
        title="HTML Preview"
        className="w-full h-[360px] border-0 bg-background"
      />
    </div>
  );
}

const PREVIEWABLE = new Set(["html", "svg", "mermaid"]);

export function CodeBlock({
  children,
  streaming,
}: {
  children: React.ReactNode;
  streaming?: boolean;
}) {
  const preRef = useRef<HTMLPreElement | null>(null);
  const [copied, setCopied] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);
  const { resolvedTheme } = useTheme();
  const isDark = resolvedTheme === "dark";

  const { language, code } = useMemo(() => getCodeDetails(children), [children]);
  const isPreviewable = PREVIEWABLE.has(language);
  const [tab, setTab] = useState<"code" | "preview">(() =>
    isPreviewable ? "preview" : "code"
  );

  async function copy() {
    const textToCopy = code || preRef.current?.textContent || "";
    try {
      await navigator.clipboard.writeText(textToCopy);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard permission denied.
    }
  }

  const previewElement = (
    <>
      {language === "mermaid" && (
        <MermaidPreview code={code} isDark={isDark} streaming={streaming} />
      )}
      {language === "svg" && <SvgPreview code={code} />}
      {language === "html" && <HtmlPreview code={code} isDark={isDark} />}
    </>
  );

  return (
    <div className="group/code my-3 rounded-lg border bg-muted/20 overflow-hidden text-xs">
      <div className="flex items-center justify-between border-b bg-muted/50 px-3 py-1.5 font-mono text-[11px] text-muted-foreground select-none">
        <div className="flex items-center gap-2">
          <span className="font-semibold uppercase tracking-wider text-[10px] text-foreground/70">
            {language || "code"}
          </span>
          {isPreviewable && (
            <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary">
              artifact
            </span>
          )}
        </div>
        <div className="flex items-center gap-1.5">
          {isPreviewable && (
            <div className="flex items-center rounded-md border bg-background/80 p-0.5 text-muted-foreground">
              <button
                type="button"
                onClick={() => setTab("code")}
                className={`flex items-center gap-1 rounded px-2 py-0.5 transition-colors cursor-pointer ${
                  tab === "code"
                    ? "bg-accent text-foreground font-medium shadow-xs"
                    : "hover:text-foreground"
                }`}
                aria-label="View code"
              >
                <CodeIcon className="size-3" />
                <span>Code</span>
              </button>
              <button
                type="button"
                onClick={() => setTab("preview")}
                className={`flex items-center gap-1 rounded px-2 py-0.5 transition-colors cursor-pointer ${
                  tab === "preview"
                    ? "bg-accent text-foreground font-medium shadow-xs"
                    : "hover:text-foreground"
                }`}
                aria-label="View preview"
              >
                <Eye className="size-3" />
                <span>Preview</span>
              </button>
            </div>
          )}

          {isPreviewable && (
            <button
              type="button"
              aria-label="Expand view"
              title="Expand view"
              onClick={() => setIsExpanded(true)}
              className="rounded-md border bg-background/80 p-1.5 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
            >
              <Maximize2 className="size-3.5" />
            </button>
          )}

          <button
            type="button"
            aria-label="Copy code"
            title="Copy code"
            onClick={() => void copy()}
            className="flex items-center gap-1 rounded-md border bg-background/80 px-2 py-1 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          >
            {copied ? (
              <>
                <Check className="size-3.5 text-emerald-600" />
                <span>Copied</span>
              </>
            ) : (
              <>
                <Copy className="size-3.5" />
                <span>Copy</span>
              </>
            )}
          </button>
        </div>
      </div>

      <div className="relative">
        {isPreviewable && tab === "preview" ? (
          <div className="p-3">{previewElement}</div>
        ) : (
          <pre ref={preRef} className="overflow-x-auto p-3 m-0 bg-transparent text-foreground">
            {children}
          </pre>
        )}
      </div>

      {isPreviewable && (
        <Dialog open={isExpanded} onOpenChange={setIsExpanded}>
          <DialogContent className="max-w-4xl max-h-[85vh] flex flex-col p-4">
            <DialogHeader className="flex flex-row items-center justify-between pb-2 border-b">
              <DialogTitle className="text-sm font-semibold uppercase tracking-wider flex items-center gap-2">
                <span>{language}</span>
                <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary">
                  artifact preview
                </span>
              </DialogTitle>
              <div className="flex items-center gap-2 mr-6">
                <div className="flex items-center rounded-md border bg-background p-0.5 text-muted-foreground">
                  <button
                    type="button"
                    onClick={() => setTab("code")}
                    className={`flex items-center gap-1 rounded px-2 py-0.5 transition-colors cursor-pointer ${
                      tab === "code"
                        ? "bg-accent text-foreground font-medium"
                        : "hover:text-foreground"
                    }`}
                  >
                    <CodeIcon className="size-3" />
                    <span>Code</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => setTab("preview")}
                    className={`flex items-center gap-1 rounded px-2 py-0.5 transition-colors cursor-pointer ${
                      tab === "preview"
                        ? "bg-accent text-foreground font-medium"
                        : "hover:text-foreground"
                    }`}
                  >
                    <Eye className="size-3" />
                    <span>Preview</span>
                  </button>
                </div>
                <button
                  type="button"
                  aria-label="Copy code"
                  onClick={() => void copy()}
                  className="flex items-center gap-1 rounded-md border bg-background px-2 py-1 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
                >
                  {copied ? (
                    <Check className="size-3.5 text-emerald-600" />
                  ) : (
                    <Copy className="size-3.5" />
                  )}
                  <span>{copied ? "Copied" : "Copy"}</span>
                </button>
              </div>
            </DialogHeader>
            <div className="flex-1 overflow-auto p-2">
              {tab === "preview" ? (
                previewElement
              ) : (
                <pre className="overflow-x-auto p-4 rounded-md bg-muted/40 font-mono text-xs text-foreground">
                  {code}
                </pre>
              )}
            </div>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
