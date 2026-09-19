import { ChevronDown, CheckCircle2, Sparkles, BookOpen, Globe, Calendar, GitBranch, MessageSquare, Terminal, FileText, Cpu } from "lucide-react";
import { Badge } from "@/components/ui/badge";

export interface ThinkingTraceProps {
  rows: string[];
}

interface HumanizedStep {
  id: string;
  label: string;
  icon: typeof Sparkles;
}

function humanizeStep(row: string, index: number): HumanizedStep {
  const lower = row.toLowerCase();

  if (lower.includes("search_documents") || lower.includes("searching documents")) {
    return {
      id: `${index}-${row}`,
      label: "Searched referenced workspace documents",
      icon: FileText,
    };
  }
  if (lower.includes("list_documents")) {
    return {
      id: `${index}-${row}`,
      label: "Reviewed document index & attachments",
      icon: BookOpen,
    };
  }
  if (lower.includes("web_fetch") || lower.includes("web search") || lower.includes("@web")) {
    return {
      id: `${index}-${row}`,
      label: "Retrieved public information from the web",
      icon: Globe,
    };
  }
  if (lower.includes("calendar")) {
    return {
      id: `${index}-${row}`,
      label: "Checked calendar schedule and events",
      icon: Calendar,
    };
  }
  if (lower.includes("github")) {
    return {
      id: `${index}-${row}`,
      label: "Queried GitHub repository data",
      icon: GitBranch,
    };
  }
  if (lower.includes("slack")) {
    return {
      id: `${index}-${row}`,
      label: "Checked communication in Slack",
      icon: MessageSquare,
    };
  }
  if (lower.includes("code_runner") || lower.includes("sandbox")) {
    return {
      id: `${index}-${row}`,
      label: "Executed calculations in code sandbox",
      icon: Terminal,
    };
  }
  if (lower.includes("compacted") || lower.includes("history summarized")) {
    return {
      id: `${index}-${row}`,
      label: "Synthesized previous conversation context",
      icon: BookOpen,
    };
  }
  if (lower.includes("model call")) {
    return {
      id: `${index}-${row}`,
      label: "Composed and structured findings",
      icon: Cpu,
    };
  }

  // Fallback to cleaned readable text without underscores or trailing ellipsis
  const cleaned = row
    .replace(/^tool_start\s+/, "")
    .replace(/^tool_end\s+/, "")
    .replace(/_/g, " ")
    .replace(/…$/, "")
    .replace(/\s+/g, " ")
    .trim();

  return {
    id: `${index}-${row}`,
    label: cleaned.charAt(0).toUpperCase() + cleaned.slice(1),
    icon: Sparkles,
  };
}

export function ThinkingTrace({ rows }: ThinkingTraceProps) {
  if (!rows || rows.length === 0) return null;

  const steps = rows.map(humanizeStep);

  return (
    <details className="group mb-2.5 text-xs">
      <summary className="flex w-fit cursor-pointer list-none items-center gap-1.5 select-none rounded-lg px-2 py-1 text-muted-foreground hover:bg-muted/60 hover:text-foreground transition-colors">
        <ChevronDown className="size-3 text-muted-foreground/70 transition-transform duration-200 group-open:rotate-0 -rotate-90" />
        <Sparkles className="size-3 text-emerald-600 dark:text-emerald-400 shrink-0" />
        <span className="font-medium text-foreground/80">Research & Steps</span>
        <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono h-4 shrink-0 ml-0.5">
          {steps.length}
        </Badge>
      </summary>

      <div className="mt-2 ml-2.5 border-l border-border/70 pl-3.5 space-y-1.5 py-0.5">
        {steps.map((step) => {
          const Icon = step.icon;
          return (
            <div key={step.id} className="flex items-center gap-2 text-foreground/75">
              <span className="flex size-4 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                <CheckCircle2 className="size-2.5" />
              </span>
              <Icon className="size-3 text-muted-foreground shrink-0" />
              <span className="text-[11px] leading-tight truncate">{step.label}</span>
            </div>
          );
        })}
      </div>
    </details>
  );
}
