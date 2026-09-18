import { Plus, ShieldCheck, SlidersHorizontal } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { splitTemplate } from "@/features/tools/toolForm";
import { templateHost, type ToolTemplate } from "@/features/tools/templates";

interface TemplatePreviewDialogProps {
  template: ToolTemplate | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onUseTemplate: (template: ToolTemplate) => void;
  onCustomize: (template: ToolTemplate) => void;
  isAdding?: boolean;
}

export function TemplatePreviewDialog({
  template,
  open,
  onOpenChange,
  onUseTemplate,
  onCustomize,
  isAdding = false,
}: TemplatePreviewDialogProps) {
  if (!template) return null;
  const Icon = template.icon;
  const host = templateHost(template.tool.urlTemplate);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogHeader className="border-b px-6 py-4 text-left">
          <div className="flex items-start gap-3">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-xl border bg-muted/60 text-foreground">
              <Icon className="size-5" />
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <DialogTitle className="text-base font-semibold">{template.title}</DialogTitle>
                <Badge variant="outline" className="text-[11px] font-normal">
                  {template.category}
                </Badge>
                <span className="rounded-md border border-emerald-600/30 bg-emerald-600/10 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-emerald-700 dark:text-emerald-400">
                  {template.tool.method}
                </span>
              </div>
              <DialogDescription className="mt-1 text-xs text-muted-foreground">
                {template.tagline}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-6 py-5">
          <div>
            <h4 className="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
              Agent Tool Description
            </h4>
            <p className="mt-1.5 rounded-lg border bg-muted/30 p-3 text-xs leading-relaxed text-foreground">
              {template.tool.description}
            </p>
          </div>

          <div>
            <div className="mb-1.5 flex items-center justify-between">
              <h4 className="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
                Endpoint ({host})
              </h4>
              <span className="text-[10px] font-mono text-muted-foreground">Public API</span>
            </div>
            <div className="break-all rounded-lg border bg-muted/30 p-3 font-mono text-xs leading-relaxed">
              {splitTemplate(template.tool.urlTemplate).map((seg, i) =>
                seg.placeholder ? (
                  <span
                    key={i}
                    className="rounded bg-amber-500/15 px-1 py-0.5 font-semibold text-amber-700 dark:text-amber-400"
                  >
                    {`{{${seg.placeholder}}}`}
                  </span>
                ) : (
                  <span key={i}>{seg.text}</span>
                ),
              )}
            </div>
          </div>

          <div>
            <h4 className="mb-2 text-xs font-semibold tracking-wider text-muted-foreground uppercase">
              Parameters ({template.tool.params.length})
            </h4>
            {template.tool.params.length === 0 ? (
              <p className="rounded-lg border border-dashed p-3 text-xs text-muted-foreground">
                No parameters required.
              </p>
            ) : (
              <div className="overflow-hidden rounded-lg border">
                <div className="grid grid-cols-[1fr_4rem_4.5rem_1.5fr] gap-2 border-b bg-muted/50 px-3 py-2 text-[11px] font-medium tracking-wider text-muted-foreground uppercase">
                  <span>Name</span>
                  <span>In</span>
                  <span>Type</span>
                  <span>Description</span>
                </div>
                <div className="divide-y divide-border/60">
                  {template.tool.params.map((p, i) => (
                    <div
                      key={i}
                      className="grid grid-cols-[1fr_4rem_4.5rem_1.5fr] items-center gap-2 px-3 py-2 text-xs"
                    >
                      <span className="font-mono font-medium text-foreground">{p.name}</span>
                      <span className="font-mono text-[11px] text-muted-foreground">{p.in}</span>
                      <span className="font-mono text-[11px] text-muted-foreground">{p.type}</span>
                      <span className="truncate text-muted-foreground" title={p.description}>
                        {p.description || "—"}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>

          <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-3.5 flex items-start gap-3">
            <ShieldCheck className="size-4 shrink-0 text-emerald-600 dark:text-emerald-400 mt-0.5" />
            <div className="text-xs">
              <p className="font-medium text-foreground">Verified & Safe</p>
              <p className="mt-0.5 text-muted-foreground">
                This is a pre-vetted, read-only HTTP API with no API key needed. You can add it directly or customize its name, description, or headers before saving.
              </p>
            </div>
          </div>
        </div>

        <DialogFooter className="border-t bg-muted/30 px-6 py-3 justify-between sm:justify-between">
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onOpenChange(false);
                onCustomize(template);
              }}
            >
              <SlidersHorizontal className="size-3.5 mr-1" /> Customize
            </Button>
            <Button
              size="sm"
              onClick={() => {
                onOpenChange(false);
                onUseTemplate(template);
              }}
              disabled={isAdding}
            >
              <Plus className="size-3.5 mr-1" />
              {isAdding ? "Adding…" : "Add to My Tools"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
