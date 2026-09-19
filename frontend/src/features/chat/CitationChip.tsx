import { FileText } from "lucide-react";
import { useState } from "react";
import { Tooltip as TooltipPrimitive } from "radix-ui";
import type { Source } from "@/lib/types";

/**
 * CitationChip is the inline [N] affordance, ChatGPT-style: a small numbered
 * badge that shows what it refers to on hover (file name, page, heading).
 * Clicking opens the source document when the page knows how (onCite), and
 * otherwise pins the preview — so touch devices get the same info.
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
  const [pinned, setPinned] = useState(false);
  const label = source
    ? `Source ${n}: ${source.title}${source.page > 0 ? `, p.${source.page}` : ""}`
    : `View source ${n}`;
  return (
    <TooltipPrimitive.Provider delayDuration={200}>
      <TooltipPrimitive.Root open={pinned}>
        <TooltipPrimitive.Trigger asChild>
          <button
            type="button"
            aria-label={label}
            onPointerEnter={() => setPinned(true)}
            onPointerLeave={() => setPinned(false)}
            onFocus={() => setPinned(true)}
            onBlur={() => setPinned(false)}
            onKeyUp={(e) => {
              if (e.key === "Escape") setPinned(false);
            }}
            onClick={() => {
              if (onCite) onCite(n);
              else setPinned((p) => !p);
            }}
            className="mx-px inline-flex h-4 min-w-4 cursor-pointer items-center justify-center rounded-[4px] border bg-muted px-1 align-super text-[9px] leading-none font-semibold text-muted-foreground transition-colors hover:border-primary/50 hover:text-primary"
          >
            {n}
          </button>
        </TooltipPrimitive.Trigger>
        {source && (
          <TooltipPrimitive.Portal>
            <TooltipPrimitive.Content
              side="top"
              sideOffset={6}
              collisionPadding={12}
              className="z-50 max-w-72 rounded-lg border bg-popover p-2.5 text-popover-foreground shadow-md"
            >
              <p className="flex items-center gap-1.5 text-xs font-medium">
                <FileText className="size-3.5 shrink-0 text-muted-foreground" />
                <span className="truncate">{source.title}</span>
              </p>
              {(source.page > 0 || source.heading) && (
                <p className="mt-1 text-muted-foreground text-[11px]">
                  {source.page > 0 && <span>Page {source.page}</span>}
                  {source.heading && (
                    <span>{source.page > 0 ? " · " : ""}§ {source.heading}</span>
                  )}
                </p>
              )}
              <TooltipPrimitive.Arrow className="fill-popover" width={10} height={5} />
            </TooltipPrimitive.Content>
          </TooltipPrimitive.Portal>
        )}
      </TooltipPrimitive.Root>
    </TooltipPrimitive.Provider>
  );
}
