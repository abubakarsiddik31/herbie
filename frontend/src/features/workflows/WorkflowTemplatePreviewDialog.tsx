import { ArrowRight, Bot, Check } from "lucide-react";
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
import { getNodeDefinition } from "./nodeTypes";
import type { WorkflowExample } from "./exampleWorkflows";

interface WorkflowTemplatePreviewDialogProps {
  template: WorkflowExample | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onUseTemplate: (template: WorkflowExample) => void;
  isCloning?: boolean;
}

export function WorkflowTemplatePreviewDialog({
  template,
  open,
  onOpenChange,
  onUseTemplate,
  isCloning,
}: WorkflowTemplatePreviewDialogProps) {
  if (!template) return null;

  const Icon = template.icon;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl lg:max-w-3xl h-[85vh] max-h-[85vh] flex flex-col p-0 overflow-hidden">
        <DialogHeader className="p-5 pb-4 border-b border-border bg-muted/20 shrink-0">
          <div className="flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-xl bg-primary/10 border border-primary/20 text-primary shrink-0">
              <Icon className="size-5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <DialogTitle className="text-base tracking-tight">{template.title}</DialogTitle>
                <Badge variant="secondary" className="px-2 py-0 text-[10px] font-mono">
                  {template.category}
                </Badge>
              </div>
              <DialogDescription className="text-xs mt-0.5">{template.tagline}</DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <div className="flex-1 min-h-0 overflow-y-auto p-5">
          <div className="space-y-5 text-xs">
            {/* Description */}
            <div className="rounded-lg border border-border/70 bg-card p-3.5 leading-relaxed text-muted-foreground">
              {template.description}
            </div>

            {/* Pipeline Steps Sequence */}
            <div className="space-y-2.5">
              <p className="font-semibold text-foreground text-xs uppercase tracking-wider text-muted-foreground/80">
                Pipeline Execution Steps ({template.nodes.length} nodes)
              </p>

              <div className="space-y-2">
                {template.nodes.map((node, index) => {
                  const def = getNodeDefinition(node.type);
                  const NodeIcon = def.icon;
                  return (
                    <div key={node.id} className="flex flex-col gap-1.5">
                      <div className="flex items-center justify-between rounded-xl border border-border bg-card p-3 shadow-2xs">
                        <div className="flex items-center gap-3">
                          <div className={`flex size-8 items-center justify-center rounded-lg border ${def.bgColor}`}>
                            <NodeIcon className={`size-4 ${def.color}`} />
                          </div>
                          <div>
                            <p className="font-semibold text-foreground text-xs">{node.name || def.label}</p>
                            <p className="text-[11px] text-muted-foreground">{def.description}</p>
                          </div>
                        </div>
                        <Badge variant="outline" className="text-[10px] font-mono capitalize">
                          {def.category}
                        </Badge>
                      </div>

                      {index < template.nodes.length - 1 && (
                        <div className="flex justify-center my-0.5">
                          <ArrowRight className="size-3 text-muted-foreground/50 rotate-90" />
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Special capabilities */}
            {template.exposeAsTool && (
              <div className="rounded-xl border border-primary/30 bg-primary/5 p-3 flex items-start gap-2.5">
                <Bot className="size-4 text-primary shrink-0 mt-0.5" />
                <div>
                  <p className="font-semibold text-primary text-xs">Preconfigured Chatbot Tool</p>
                  <p className="text-[11px] text-muted-foreground mt-0.5">
                    This workflow is ready to be called by your AI assistant in chat as <code className="font-mono bg-muted px-1 rounded">{template.toolName}</code>.
                  </p>
                </div>
              </div>
            )}
          </div>
        </div>

        <DialogFooter className="p-4 border-t border-border bg-card/60 shrink-0">
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          <Button
            size="sm"
            onClick={() => onUseTemplate(template)}
            disabled={isCloning}
            className="gap-2 shadow-xs"
          >
            <Check className="size-3.5" />
            <span>{isCloning ? "Cloning..." : "Use This Template"}</span>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
