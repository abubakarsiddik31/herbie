import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ChatMessage } from "@/lib/types";

export function BranchSwitcher({
  branch,
  disabled = false,
  onActivate,
}: {
  branch: NonNullable<ChatMessage["branch"]>;
  disabled?: boolean;
  onActivate: (messageId: string) => void;
}) {
  if (branch.count <= 1) return null;
  const atStart = branch.index <= 0;
  const atEnd = branch.index >= branch.count - 1;
  return (
    <div className="flex items-center gap-0.5 text-[11px] text-muted-foreground/60">
      <Button
        variant="ghost"
        size="icon-xs"
        className="size-5 text-muted-foreground/60 hover:text-foreground"
        aria-label="Previous branch"
        title="Previous branch"
        disabled={disabled || atStart}
        onClick={() => onActivate(branch.ids[branch.index - 1])}
      >
        <ChevronLeft className="size-3.5" />
      </Button>
      <span aria-live="polite">
        {branch.index + 1}/{branch.count}
      </span>
      <Button
        variant="ghost"
        size="icon-xs"
        className="size-5 text-muted-foreground/60 hover:text-foreground"
        aria-label="Next branch"
        title="Next branch"
        disabled={disabled || atEnd}
        onClick={() => onActivate(branch.ids[branch.index + 1])}
      >
        <ChevronRight className="size-3.5" />
      </Button>
    </div>
  );
}
