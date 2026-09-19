import { useEffect, useState } from "react";
import {
  Activity,
  BookOpen,
  Calendar,
  FileText,
  GitBranch,
  Globe,
  MessageSquare,
  Terminal,
} from "lucide-react";
import { cn } from "@/lib/utils";

export interface RunLoaderProps {
  trace?: string[];
  className?: string;
}

function getContextualAction(trace?: string[], elapsed: number = 0): {
  verbiage: string;
  Icon: typeof Activity;
} {
  if (trace && trace.length > 0) {
    const latest = trace[trace.length - 1].toLowerCase();
    if (latest.includes("search_documents") || latest.includes("searching documents") || latest.includes("document")) {
      return { verbiage: "Reading through referenced documents...", Icon: FileText };
    }
    if (latest.includes("web_fetch") || latest.includes("web search") || latest.includes("@web")) {
      return { verbiage: "Retrieving relevant information from the web...", Icon: Globe };
    }
    if (latest.includes("calendar")) {
      return { verbiage: "Checking calendar schedule...", Icon: Calendar };
    }
    if (latest.includes("github")) {
      return { verbiage: "Inspecting GitHub repository...", Icon: GitBranch };
    }
    if (latest.includes("slack")) {
      return { verbiage: "Checking Slack updates...", Icon: MessageSquare };
    }
    if (latest.includes("code_runner") || latest.includes("sandbox")) {
      return { verbiage: "Running calculations in sandbox...", Icon: Terminal };
    }
    if (latest.includes("compacted") || latest.includes("history summarized")) {
      return { verbiage: "Reviewing earlier conversation...", Icon: BookOpen };
    }
  }

  // Dynamic progression based on time
  if (elapsed < 2.5) {
    return { verbiage: "Analyzing your request...", Icon: Activity };
  }
  if (elapsed < 5.5) {
    return { verbiage: "Gathering context & insights...", Icon: Activity };
  }
  if (elapsed < 8.5) {
    return { verbiage: "Connecting the details...", Icon: Activity };
  }
  if (elapsed < 12) {
    return { verbiage: "Formulating a thoughtful response...", Icon: Activity };
  }
  return { verbiage: "Polishing the final answer...", Icon: Activity };
}

export function RunLoader({ trace, className }: RunLoaderProps) {
  const [elapsed, setElapsed] = useState(0);

  useEffect(() => {
    const t = setInterval(() => setElapsed((s) => s + 1), 1000);
    return () => clearInterval(t);
  }, []);

  const { verbiage, Icon } = getContextualAction(trace, elapsed);

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

      {/* Engaging, Human-Friendly Dynamic Verbiage with Authentic Tool/Activity Icons */}
      <div className="flex items-center gap-1.5 min-w-0">
        <Icon className="size-3 text-emerald-600 dark:text-emerald-400 shrink-0" />
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
