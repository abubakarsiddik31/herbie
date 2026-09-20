import { ExternalLink, FileText, Globe } from "lucide-react";
import { useState } from "react";
import { Tooltip as TooltipPrimitive } from "radix-ui";
import { cn } from "@/lib/utils";
import type { Source } from "@/lib/types";

/**
 * CitationChip is the inline [N] affordance: a numbered badge showing whether
 * the citation is from Web Search or a Document.
 * On hover, it displays a rich preview with source kind badge, title, domain, snippet, and link.
 */
export function CitationChip({
  n,
  source,
  onCite,
}: {
  n: number;
  source?: Source;
  onCite?: (n: number) => void;
}) {
  const [open, setOpen] = useState(false);

  const rawUrl =
    source?.url ||
    (source?.documentId?.startsWith("http://") || source?.documentId?.startsWith("https://")
      ? source.documentId
      : undefined) ||
    (source?.heading?.startsWith("http://") || source?.heading?.startsWith("https://")
      ? source.heading
      : undefined);

  let domain = "";
  if (rawUrl) {
    try {
      domain = new URL(rawUrl).hostname.replace(/^www\./, "");
    } catch {
      domain = rawUrl;
    }
  }

  const isDocument = Boolean(
    (source?.documentId && !source.documentId.startsWith("http://") && !source.documentId.startsWith("https://")) ||
    (source && (source.page > 0 || (source.heading && !source.heading.startsWith("http://") && !source.heading.startsWith("https://"))))
  );
  const isWeb = !isDocument;

  const displayTitle = source?.title || (rawUrl ? domain || rawUrl : `Source ${n}`);
  const label = source
    ? `Source ${n}: ${displayTitle}${source.page > 0 ? `, p.${source.page}` : ""}`
    : `Source ${n}`;

  function handleClick() {
    if (onCite) {
      onCite(n);
    } else if (rawUrl) {
      window.open(rawUrl, "_blank", "noopener,noreferrer");
    } else {
      setOpen((o) => !o);
    }
  }

  return (
    <TooltipPrimitive.Provider delayDuration={150}>
      <TooltipPrimitive.Root open={open} onOpenChange={setOpen}>
        <TooltipPrimitive.Trigger asChild>
          <button
            type="button"
            aria-label={label}
            onClick={handleClick}
            className={cn(
              "mx-0.5 inline-flex h-4 min-w-4 cursor-pointer items-center justify-center rounded-[4px] border px-1 align-super text-[9px] leading-none font-semibold transition-all active:scale-95",
              isWeb
                ? "border-sky-500/30 bg-sky-500/10 text-sky-700 dark:text-sky-300 hover:border-sky-500/60 hover:bg-sky-500/20 hover:text-sky-800 dark:hover:text-sky-100"
                : "border-border/70 bg-muted/80 text-muted-foreground hover:border-primary/50 hover:bg-muted hover:text-primary"
            )}
          >
            {n}
          </button>
        </TooltipPrimitive.Trigger>
        <TooltipPrimitive.Portal>
          <TooltipPrimitive.Content
            side="top"
            sideOffset={6}
            collisionPadding={12}
            className="z-50 max-w-84 rounded-xl border border-border/80 bg-popover p-3 text-popover-foreground shadow-lg transition-all animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95"
          >
            {/* Header: Source type badge (Web Search vs Document) + Source number */}
            <div className="flex items-center justify-between gap-2 border-b border-border/50 pb-2 mb-2">
              <span
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[10px] font-medium tracking-tight",
                  isWeb
                    ? "bg-sky-500/15 text-sky-700 dark:text-sky-300 border border-sky-500/25"
                    : "bg-muted text-muted-foreground border border-border/60"
                )}
              >
                {isWeb ? (
                  <>
                    <Globe className="size-3 text-sky-500" />
                    <span>Web Search</span>
                  </>
                ) : (
                  <>
                    <FileText className="size-3 text-muted-foreground" />
                    <span>Document</span>
                  </>
                )}
              </span>
              <span className="font-mono text-[10px] text-muted-foreground/80 font-medium">
                [{n}]
              </span>
            </div>

            {/* Content: Title & Domain */}
            <div className="space-y-1.5">
              <div>
                <p className="font-semibold text-xs leading-snug line-clamp-2 text-foreground">
                  {displayTitle}
                </p>
                {domain && (
                  <p className="mt-0.5 flex items-center gap-1 font-mono text-[10px] text-muted-foreground">
                    <Globe className="size-2.5 text-muted-foreground/70" />
                    <span>{domain}</span>
                  </p>
                )}
              </div>

              {/* Document metadata (page, section) */}
              {source && !isWeb && (source.page > 0 || source.heading) && (
                <p className="text-[11px] text-muted-foreground">
                  {source.page > 0 && <span>Page {source.page}</span>}
                  {source.heading && (
                    <span>{source.page > 0 ? " · " : ""}§ {source.heading}</span>
                  )}
                </p>
              )}

              {/* Excerpt / Snippet */}
              {source?.snippet && (
                <p className="text-[11px] text-muted-foreground/90 leading-relaxed line-clamp-3 pt-1.5 border-t border-border/40">
                  {source.snippet}
                </p>
              )}

              {/* Action */}
              {rawUrl ? (
                <div className="pt-1.5 border-t border-border/40">
                  <a
                    href={rawUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={(e) => e.stopPropagation()}
                    className="inline-flex items-center gap-1.5 text-xs font-medium text-sky-600 hover:text-sky-700 dark:text-sky-400 dark:hover:text-sky-300 hover:underline"
                  >
                    <span>Visit website</span>
                    <ExternalLink className="size-3" />
                  </a>
                </div>
              ) : onCite ? (
                <div className="pt-1.5 border-t border-border/40">
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      onCite(n);
                    }}
                    className="inline-flex items-center gap-1.5 text-xs font-medium text-primary hover:underline"
                  >
                    <span>View document</span>
                    <ExternalLink className="size-3" />
                  </button>
                </div>
              ) : null}
            </div>

            <TooltipPrimitive.Arrow className="fill-popover" width={10} height={5} />
          </TooltipPrimitive.Content>
        </TooltipPrimitive.Portal>
      </TooltipPrimitive.Root>
    </TooltipPrimitive.Provider>
  );
}
