import { useState } from "react";
import { Copy, Settings2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { Workflow } from "@/lib/types";

interface WorkflowSettingsDialogProps {
  workflow: Workflow;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSave: (patch: Partial<Workflow>) => void;
}

export function WorkflowSettingsDialog({
  workflow,
  open,
  onOpenChange,
  onSave,
}: WorkflowSettingsDialogProps) {
  const [name, setName] = useState(workflow.name);
  const [description, setDescription] = useState(workflow.description);
  const [isActive, setIsActive] = useState(workflow.isActive);
  const [triggerType, setTriggerType] = useState(workflow.triggerType || "manual");
  const [webhookSlug, setWebhookSlug] = useState(workflow.webhookSlug || "");
  const [webhookSecret, setWebhookSecret] = useState(workflow.webhookSecret || "");
  const [exposeAsTool, setExposeAsTool] = useState(workflow.exposeAsTool);
  const [toolName, setToolName] = useState(workflow.toolName || "");
  const [toolDescription, setToolDescription] = useState(workflow.toolDescription || "");

  const apiBase = window.location.origin;
  const webhookUrl = webhookSlug ? `${apiBase}/api/webhooks/${webhookSlug}` : "";

  function copyWebhookUrl() {
    if (!webhookUrl) return;
    navigator.clipboard.writeText(webhookUrl);
    toast.success("Webhook URL copied to clipboard");
  }

  function handleSave() {
    if (!name.trim()) {
      toast.error("Workflow name is required");
      return;
    }
    const cleanSlug = webhookSlug.trim() ? webhookSlug.trim().toLowerCase().replace(/[^a-z0-9-_]/g, "-") : null;
    onSave({
      name: name.trim(),
      description: description.trim(),
      isActive,
      triggerType,
      webhookSlug: cleanSlug,
      webhookSecret: webhookSecret.trim(),
      exposeAsTool,
      toolName: toolName.trim(),
      toolDescription: toolDescription.trim(),
    });
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-base">
            <Settings2 className="size-4 text-primary" />
            <span>Workflow Settings</span>
          </DialogTitle>
          <DialogDescription className="text-xs">
            Configure triggers, webhook URLs, and chatbot agent tool integration.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2 text-xs">
          <div className="space-y-1.5">
            <Label htmlFor="wf-name" className="text-xs">Workflow Name</Label>
            <Input
              id="wf-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Daily GitHub Digest"
              className="h-8 text-xs"
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="wf-desc" className="text-xs">Description</Label>
            <Input
              id="wf-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What this workflow automates..."
              className="h-8 text-xs"
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="wf-trigger" className="text-xs">Primary Trigger Type</Label>
            <select
              id="wf-trigger"
              value={triggerType}
              onChange={(e) => setTriggerType(e.target.value)}
              className="h-8 w-full rounded-md border border-input bg-background px-2.5 text-xs outline-none focus-visible:border-ring"
            >
              <option value="manual">Manual Execution</option>
              <option value="webhook">Incoming Webhook</option>
              <option value="chat_agent">Chat Agent Tool</option>
            </select>
          </div>

          <div className="flex items-center justify-between rounded-lg border border-border p-3">
            <div>
              <p className="font-semibold text-foreground">Active Status</p>
              <p className="text-[11px] text-muted-foreground">
                When active, incoming webhooks and agent tool calls are processed.
              </p>
            </div>
            <Switch checked={isActive} onCheckedChange={setIsActive} />
          </div>

          {/* Webhook Configuration */}
          <div className="space-y-3 rounded-lg border border-border p-3">
            <p className="font-semibold text-foreground">Webhook Integration</p>

            <div className="space-y-1">
              <Label htmlFor="wf-slug" className="text-xs">Webhook Slug</Label>
              <Input
                id="wf-slug"
                value={webhookSlug}
                onChange={(e) => setWebhookSlug(e.target.value)}
                placeholder="e.g. github-alerts"
                className="h-8 font-mono text-xs"
              />
            </div>

            {webhookUrl && (
              <div className="space-y-1">
                <Label className="text-xs">Public Webhook URL</Label>
                <div className="flex gap-1.5">
                  <Input readOnly value={webhookUrl} className="h-8 font-mono text-[11px] bg-muted/40" />
                  <Button variant="outline" size="icon-xs" onClick={copyWebhookUrl} className="size-8 shrink-0">
                    <Copy className="size-3.5" />
                  </Button>
                </div>
              </div>
            )}

            <div className="space-y-1">
              <Label htmlFor="wf-secret" className="text-xs">Webhook Secret Token (optional)</Label>
              <Input
                id="wf-secret"
                type="password"
                value={webhookSecret}
                onChange={(e) => setWebhookSecret(e.target.value)}
                placeholder="Secret passed via X-Webhook-Secret header"
                className="h-8 font-mono text-xs"
              />
            </div>
          </div>

          {/* Chatbot Agent Tool Bridge */}
          <div className="space-y-3 rounded-lg border border-border p-3">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-semibold text-foreground">Expose as Agent Tool</p>
                <p className="text-[11px] text-muted-foreground">
                  The chatbot agent can automatically invoke this workflow during chat!
                </p>
              </div>
              <Switch checked={exposeAsTool} onCheckedChange={setExposeAsTool} />
            </div>

            {exposeAsTool && (
              <>
                <div className="space-y-1">
                  <Label htmlFor="tool-name" className="text-xs">Tool Name</Label>
                  <Input
                    id="tool-name"
                    value={toolName}
                    onChange={(e) => setToolName(e.target.value)}
                    placeholder="e.g. run_daily_sync"
                    className="h-8 font-mono text-xs"
                  />
                </div>

                <div className="space-y-1">
                  <Label htmlFor="tool-desc" className="text-xs">Model Description</Label>
                  <Input
                    id="tool-desc"
                    value={toolDescription}
                    onChange={(e) => setToolDescription(e.target.value)}
                    placeholder="Explain when the model should call this workflow..."
                    className="h-8 text-xs"
                  />
                </div>
              </>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button size="sm" onClick={handleSave}>
            Save Changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
