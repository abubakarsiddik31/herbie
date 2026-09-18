import { useState } from "react";
import { Search } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { NODE_DEFINITIONS, type NodeDefinition } from "./nodeTypes";

interface NodePaletteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelectNode: (def: NodeDefinition) => void;
}

const CATEGORIES = [
  { id: "all", label: "All Nodes" },
  { id: "trigger", label: "Triggers" },
  { id: "tool", label: "External Tools" },
  { id: "ai", label: "AI & Models" },
  { id: "logic", label: "Logic & Flow" },
  { id: "output", label: "Outputs" },
] as const;

export function NodePaletteDialog({ open, onOpenChange, onSelectNode }: NodePaletteDialogProps) {
  const [search, setSearch] = useState("");
  const [activeCategory, setActiveCategory] = useState<string>("all");

  const filtered = NODE_DEFINITIONS.filter((n) => {
    const matchesCategory = activeCategory === "all" || n.category === activeCategory;
    const q = search.toLowerCase().trim();
    const matchesSearch =
      !q ||
      n.label.toLowerCase().includes(q) ||
      n.description.toLowerCase().includes(q) ||
      n.type.toLowerCase().includes(q);
    return matchesCategory && matchesSearch;
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl gap-0 p-0 overflow-hidden">
        <DialogHeader className="p-4 pb-3 border-b border-border">
          <DialogTitle className="text-base tracking-tight">Add Workflow Step</DialogTitle>
          <DialogDescription className="text-xs">
            Choose a trigger, integration action, AI prompt, or logic block to add to your flow.
          </DialogDescription>
          <div className="relative mt-2">
            <Search className="absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground/70" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search nodes (e.g. webhook, http, github, condition)..."
              className="h-9 pl-8 text-xs"
              autoFocus
            />
          </div>
        </DialogHeader>

        {/* Category Tabs */}
        <div className="flex border-b border-border/60 px-4 pt-2 gap-1.5 overflow-x-auto text-xs">
          {CATEGORIES.map((c) => (
            <button
              key={c.id}
              type="button"
              onClick={() => setActiveCategory(c.id)}
              className={cn(
                "rounded-md px-2.5 py-1 text-xs font-medium transition-colors whitespace-nowrap mb-2",
                activeCategory === c.id
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground"
              )}
            >
              {c.label}
            </button>
          ))}
        </div>

        {/* Node Grid */}
        <ScrollArea className="max-h-[380px] p-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-2.5">
            {filtered.map((def) => {
              const Icon = def.icon;
              return (
                <button
                  key={def.type}
                  type="button"
                  onClick={() => {
                    onSelectNode(def);
                    onOpenChange(false);
                  }}
                  className="flex items-start gap-3 rounded-xl border border-border bg-card p-3 text-left transition-all hover:border-primary/50 hover:bg-muted/40 hover:shadow-xs group cursor-pointer"
                >
                  <div className={cn("flex size-9 shrink-0 items-center justify-center rounded-lg border", def.bgColor)}>
                    <Icon className={cn("size-4.5", def.color)} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-xs font-semibold text-foreground group-hover:text-primary transition-colors">
                      {def.label}
                    </p>
                    <p className="text-[11px] text-muted-foreground line-clamp-2 mt-0.5 leading-relaxed">
                      {def.description}
                    </p>
                  </div>
                </button>
              );
            })}
            {filtered.length === 0 && (
              <div className="col-span-2 py-8 text-center text-xs text-muted-foreground">
                No matching nodes found.
              </div>
            )}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
