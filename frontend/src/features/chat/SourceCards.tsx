import { FileText, ChevronDown } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { Source } from "@/lib/types";
import { cn } from "@/lib/utils";

/** A citation click from the answer text: 1-based card number + sequence so
 *  repeat clicks on the same card re-trigger the jump. */
export interface CiteJump {
  n: number;
  seq: number;
}

/**
 * SourceCards renders the document chunks a grounded answer drew from.
 * Collapsed by default: the bracket citations in the text are the primary
 * affordance, the cards are the receipt.
 */
export function SourceCards({ sources, jump }: { sources: Source[]; jump?: CiteJump | null }) {
  const [open, setOpen] = useState(false);
  const [flash, setFlash] = useState<number | null>(null);
  const itemsRef = useRef<(HTMLLIElement | null)[]>([]);

  // A citation jump opens the list, scrolls the card into view, and flashes
  // it. The flash applies immediately (it is just state); the scroll waits a
  // tick so the freshly opened list has laid out.
  useEffect(() => {
    if (!jump || jump.n < 1 || jump.n > sources.length) return;
    const idx = jump.n - 1;
    setOpen(true);
    setFlash(idx);
    const clear = setTimeout(() => setFlash(null), 1600);
    const scroll = setTimeout(() => {
      itemsRef.current[idx]?.scrollIntoView?.({ block: "nearest", behavior: "smooth" });
    }, 60);
    return () => {
      clearTimeout(clear);
      clearTimeout(scroll);
    };
  }, [jump, sources.length]);

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
            <li
              key={`${s.documentId}-${i}`}
              ref={(el) => {
                itemsRef.current[i] = el;
              }}
              className={cn("rounded-md border bg-background p-2", flash === i && "ring-2 ring-primary")}
            >
              <p className="flex items-center gap-1.5 text-xs font-medium">
                <span className="rounded bg-muted px-1 font-mono text-[10px]">[{i + 1}]</span>
                <span className="truncate" title={s.title}>{s.title}</span>
              </p>
              {(s.heading || s.page > 0) && (
                <p className="mt-0.5 text-muted-foreground text-[11px]">
                  {s.heading && <span>§ {s.heading}</span>}
                  {s.page > 0 && <span> · p.{s.page}</span>}
                </p>
              )}
              <p className="mt-1 line-clamp-3 text-muted-foreground text-xs leading-relaxed">{s.snippet}</p>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
