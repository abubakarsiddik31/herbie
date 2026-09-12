import { FileText, ChevronDown } from "lucide-react";
import { useState } from "react";
import type { Source } from "@/lib/types";
import { cn } from "@/lib/utils";

/**
 * SourceCards renders the document chunks a grounded answer drew from.
 * Collapsed by default: the bracket citations in the text are the primary
 * affordance, the cards are the receipt.
 */
export function SourceCards({ sources }: { sources: Source[] }) {
  const [open, setOpen] = useState(false);
  if (sources.length === 0) return null;
  return (
    <div className="rounded-lg border bg-muted/30">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex w-full items-center gap-1.5 px-3 py-2 text-left text-muted-foreground text-xs hover:text-foreground"
      >
        <FileText className="size-3.5" />
        Sources ({sources.length})
        <ChevronDown className={cn("ml-auto size-3.5 transition-transform", open && "rotate-180")} />
      </button>
      {open && (
        <ul className="space-y-2 px-3 pb-3">
          {sources.map((s, i) => (
            <li key={`${s.documentId}-${i}`} className="rounded-md border bg-background p-2">
              <p className="flex items-center gap-1.5 text-xs font-medium">
                <span className="rounded bg-muted px-1 font-mono text-[10px]">[{i + 1}]</span>
                <span className="truncate" title={s.title}>{s.title}</span>
              </p>
              <p className="mt-1 line-clamp-3 text-muted-foreground text-xs leading-relaxed">{s.snippet}</p>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
