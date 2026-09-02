import { useRef, useState } from "react";
import { Check, Copy } from "lucide-react";

// CodeBlock wraps a markdown code fence with a copy button that grabs the
// rendered text content of the block.
export function CodeBlock({ children }: { children: React.ReactNode }) {
  const preRef = useRef<HTMLPreElement | null>(null);
  const [copied, setCopied] = useState(false);

  async function copy() {
    const text = preRef.current?.textContent ?? "";
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard permission denied; nothing sensible to do.
    }
  }

  return (
    <div className="group/code relative">
      <button
        type="button"
        aria-label="Copy code"
        onClick={() => void copy()}
        className="absolute top-2 right-2 rounded-md border bg-background/80 p-1.5 text-muted-foreground opacity-0 backdrop-blur transition-opacity
          group-hover/code:opacity-100 focus-visible:opacity-100 hover:text-foreground"
      >
        {copied ? <Check className="size-3.5 text-emerald-600" /> : <Copy className="size-3.5" />}
      </button>
      <pre ref={preRef}>{children}</pre>
    </div>
  );
}
