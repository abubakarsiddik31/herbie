import { Bot } from "lucide-react";

import { cn } from "@/lib/utils";

export function BrandMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "flex shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground",
        className,
      )}
    >
      <Bot className="size-4" />
    </span>
  );
}
