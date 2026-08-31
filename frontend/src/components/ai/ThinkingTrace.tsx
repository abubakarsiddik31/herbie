// Adapted from beautifului.dev (MIT) by TurboProduct
import { ChevronDown } from "lucide-react";

export function ThinkingTrace({ rows }: { rows: string[] }) {
  return (
    <details className="group mb-2 text-muted-foreground text-xs">
      <summary className="flex w-fit cursor-pointer list-none items-center gap-1 select-none hover:text-foreground">
        <ChevronDown className="size-3.5 transition-transform group-open:rotate-0 -rotate-90" />
        thinking{rows.length > 0 ? ` · ${rows.length}` : ""}
      </summary>
      <ul className="mt-1.5 ml-2 space-y-0.5 border-l pl-3">
        {rows.map((row, i) => (
          <li key={`${i}-${row}`} className="font-mono">
            {row}
          </li>
        ))}
      </ul>
    </details>
  );
}
