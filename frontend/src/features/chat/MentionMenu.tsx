import { useEffect, useRef } from "react";
import {
  Calendar,
  GitBranch,
  Globe,
  MessageSquare,
  Workflow,
  Wrench,
  CheckCircle2,
  Plug,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { ChatApp } from "./useChatApps";

export interface MentionMenuProps {
  apps: ChatApp[];
  selectedIndex: number;
  onSelect: (app: ChatApp) => void;
  onHoverIndex?: (index: number) => void;
  onClose: () => void;
  className?: string;
}

export function MentionAppIcon({ name, className }: { name: ChatApp["iconName"]; className?: string }) {
  const iconClass = cn("size-4 shrink-0", className);
  switch (name) {
    case "calendar":
      return <Calendar className={cn(iconClass, "text-sky-500 dark:text-sky-400")} />;
    case "github":
      return <GitBranch className={cn(iconClass, "text-neutral-700 dark:text-neutral-300")} />;
    case "slack":
      return <MessageSquare className={cn(iconClass, "text-emerald-600 dark:text-emerald-400")} />;
    case "globe":
      return <Globe className={cn(iconClass, "text-cyan-600 dark:text-cyan-400")} />;
    case "workflow":
      return <Workflow className={cn(iconClass, "text-purple-600 dark:text-purple-400")} />;
    case "wrench":
    default:
      return <Wrench className={cn(iconClass, "text-amber-600 dark:text-amber-400")} />;
  }
}

export function MentionMenu({
  apps,
  selectedIndex,
  onSelect,
  onHoverIndex,
  onClose,
  className,
}: MentionMenuProps) {
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (listRef.current) {
      const activeEl = listRef.current.children[selectedIndex] as HTMLElement | undefined;
      if (activeEl && typeof activeEl.scrollIntoView === "function") {
        activeEl.scrollIntoView({ block: "nearest" });
      }
    }
  }, [selectedIndex]);

  if (apps.length === 0) {
    return (
      <div
        className={cn(
          "absolute bottom-full left-0 z-50 mb-2 w-72 rounded-xl border border-border/80 bg-popover/95 p-3 text-xs shadow-xl backdrop-blur-md",
          className,
        )}
      >
        <p className="text-muted-foreground text-center">No apps or tools matching query</p>
      </div>
    );
  }

  return (
    <div
      role="listbox"
      aria-label="Apps and integrations"
      className={cn(
        "absolute bottom-full left-0 z-50 mb-2 w-80 sm:w-96 rounded-2xl border border-border/80 bg-popover/95 p-1.5 shadow-2xl backdrop-blur-md animate-in fade-in slide-in-from-bottom-2 duration-150",
        className,
      )}
    >
      <div className="flex items-center justify-between px-2.5 py-1.5 border-b border-border/40 text-[11px] font-semibold text-muted-foreground">
        <span>Apps & Integrations</span>
        <span>Type to filter</span>
      </div>

      <div ref={listRef} className="max-h-64 overflow-y-auto py-1 space-y-0.5">
        {apps.map((app, idx) => {
          const isSelected = idx === selectedIndex;
          return (
            <button
              key={app.id}
              type="button"
              role="option"
              aria-selected={isSelected}
              onMouseEnter={() => onHoverIndex?.(idx)}
              onClick={(e) => {
                e.preventDefault();
                onSelect(app);
              }}
              className={cn(
                "flex w-full items-start gap-2.5 rounded-xl px-2.5 py-2 text-left text-xs transition-colors",
                isSelected
                  ? "bg-accent text-accent-foreground shadow-2xs"
                  : "text-foreground hover:bg-muted/50",
              )}
            >
              <div className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-lg border border-border/60 bg-muted/40">
                <MentionAppIcon name={app.iconName} />
              </div>

              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5">
                  <span className="font-semibold text-foreground truncate">{app.name}</span>
                  <span className="font-mono text-[10px] text-muted-foreground">@{app.mention}</span>
                </div>
                <p className="text-[11px] text-muted-foreground truncate">{app.description}</p>
              </div>

              <div className="shrink-0 self-center">
                {app.requiresConnection ? (
                  app.connected ? (
                    <Badge
                      variant="outline"
                      className="border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400 text-[10px] gap-1 px-1.5 py-0 font-normal"
                    >
                      <CheckCircle2 className="size-2.5" />
                      <span>Ready</span>
                    </Badge>
                  ) : (
                    <Badge
                      variant="outline"
                      className="border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400 text-[10px] gap-1 px-1.5 py-0 font-normal"
                    >
                      <Plug className="size-2.5" />
                      <span>Connect</span>
                    </Badge>
                  )
                ) : (
                  <Badge variant="secondary" className="text-[10px] px-1.5 py-0 font-normal text-muted-foreground">
                    {app.type === "workflow" ? "Workflow" : "Built-in"}
                  </Badge>
                )}
              </div>
            </button>
          );
        })}
      </div>

      <div className="flex items-center justify-between border-t border-border/40 px-2.5 py-1 text-[10px] text-muted-foreground">
        <span>↑↓ navigate</span>
        <span>↵ select</span>
        <button
          type="button"
          onClick={onClose}
          className="hover:text-foreground underline underline-offset-2"
        >
          esc dismiss
        </button>
      </div>
    </div>
  );
}
