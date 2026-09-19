import { useEffect, useState } from "react";
import { Sparkles } from "lucide-react";
import { cn } from "@/lib/utils";

export interface RunLoaderProps {
  trace?: string[];
  className?: string;
}

function getContextualVerbiage(trace?: string[], elapsed: number = 0): string {
  if (trace && trace.length > 0) {
    const latest = trace[trace.length - 1].toLowerCase();
    if (latest.includes("search_documents") || latest.includes("searching documents") || latest.includes("document")) {
      return "Reading through referenced documents...";
    }
    if (latest.includes("web_fetch") || latest.includes("web search") || latest.includes("@web")) {
      return "Retrieving relevant information from the web...";
    }
    if (latest.includes("calendar")) {
      return "Checking calendar schedule...";
    }
    if (latest.includes("github")) {
      return "Inspecting GitHub repository...";
    }
    if (latest.includes("slack")) {
      return "Checking Slack updates...";
    }
    if (latest.includes("code_runner") || latest.includes("sandbox")) {
      return "Running calculations in sandbox...";
    }
    if (latest.includes("compacted") || latest.includes("history summarized")) {
      return "Reviewing earlier conversation...";
    }
  }

  // Dynamic progression based on time
  if (elapsed < 2.5) {
    return "Analyzing your request...";
  }
  if (elapsed < 5.5) {
    return "Gathering context & insights...";
  }
  if (elapsed < 8.5) {
    return "Connecting the details...";
  }
  if (elapsed < 12) {
    return "Formulating a thoughtful response...";
  }
  return "Polishing the final answer...";
}

export function RunLoader({ trace, className }: RunLoaderProps) {
  const [elapsed, setElapsed] = useState(0);

  useEffect(() => {
    const t = setInterval(() => setElapsed((s) => s + 1), 1000);
    return () => clearInterval(t);
  }, []);

  const verbiage = getContextualVerbiage(trace, elapsed);

  return (
    <div className={cn("inline-flex items-center gap-2.5 py-1 text-muted-foreground", className)}>
      {/* 3 Playful Bouncing Emerald Dots */}
      <span className="flex items-center gap-1 shrink-0">
        {[0, 1, 2].map((i) => (
          <span
            key={i}
            className="size-1.5 rounded-full bg-emerald-500/80 dark:bg-emerald-400 animate-bounce"
            style={{ animationDelay: `${i * 160}ms` }}
          />
        ))}
      </span>

      {/* Engaging, Human-Friendly Dynamic Verbiage */}
      <div className="flex items-center gap-1.5 min-w-0">
        <Sparkles className="size-3 text-emerald-500/80 dark:text-emerald-400 shrink-0 animate-pulse" />
        <span className="text-xs font-medium text-foreground/80 truncate">
          {verbiage}
        </span>
      </div>

      {elapsed > 2 && (
        <span className="text-[10px] font-mono text-muted-foreground/60 shrink-0">
          · {elapsed}s
        </span>
      )}
    </div>
  );
}
