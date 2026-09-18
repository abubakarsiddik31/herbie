import { useState } from "react";
import {
  Check,
  Code2,
  Copy,
  Pencil,
  ShieldCheck,
  Terminal,
  Trash2,
} from "lucide-react";
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
import { Switch } from "@/components/ui/switch";
import type { UserTool } from "@/lib/types";
import {
  generateAgentSchema,
  generateCurlSnippet,
  HEADER_MASK,
  splitTemplate,
} from "@/features/tools/toolForm";
import { templateHost } from "@/features/tools/templates";

const METHOD_STYLES: Record<string, string> = {
  GET: "border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400",
  POST: "border-sky-600/30 bg-sky-600/10 text-sky-700 dark:text-sky-400",
  PUT: "border-amber-600/30 bg-amber-600/10 text-amber-700 dark:text-amber-400",
  PATCH: "border-violet-600/30 bg-violet-600/10 text-violet-700 dark:text-violet-400",
  DELETE: "border-rose-600/30 bg-rose-600/10 text-rose-700 dark:text-rose-400",
};

interface ToolDetailsDialogProps {
  tool: UserTool | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onEdit: (tool: UserTool) => void;
  onDelete: (tool: UserTool) => void;
  onToggle: (tool: UserTool, enabled: boolean) => void;
  onDuplicate: (tool: UserTool) => void;
}

export function ToolDetailsDialog({
  tool,
  open,
  onOpenChange,
  onEdit,
  onDelete,
  onToggle,
  onDuplicate,
}: ToolDetailsDialogProps) {
  const [activeTab, setActiveTab] = useState<"overview" | "schema" | "curl">("overview");
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  if (!tool) return null;

  const copyToClipboard = (text: string, key: string) => {
    void navigator.clipboard.writeText(text);
    setCopiedKey(key);
    setTimeout(() => {
      setCopiedKey((curr) => (curr === key ? null : curr));
    }, 2000);
  };

  const schemaJson = JSON.stringify(generateAgentSchema(tool), null, 2);
  const curlCode = generateCurlSnippet(tool);
  const host = templateHost(tool.urlTemplate);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
        <DialogHeader className="border-b px-6 py-4 text-left">
          <div className="flex flex-wrap items-center gap-2">
            <span
              className={`rounded-md border px-2 py-0.5 font-mono text-xs font-semibold ${
                METHOD_STYLES[tool.method] ?? "border-border bg-muted text-muted-foreground"
              }`}
            >
              {tool.method}
            </span>
            <DialogTitle className="font-mono text-base font-semibold">
              {tool.name}
            </DialogTitle>
            <Button
              variant="ghost"
              size="icon-xs"
              className="h-6 w-6 text-muted-foreground hover:text-foreground"
              onClick={() => copyToClipboard(tool.name, "name")}
              aria-label="Copy tool name"
              title="Copy tool name"
            >
              {copiedKey === "name" ? <Check className="size-3 text-emerald-600" /> : <Copy className="size-3" />}
            </Button>
            <div className="ml-auto flex items-center gap-2">
              <Badge
                variant={tool.enabled ? "default" : "secondary"}
                className={`text-[11px] font-medium ${
                  tool.enabled
                    ? "bg-emerald-600/15 text-emerald-700 hover:bg-emerald-600/20 dark:bg-emerald-500/20 dark:text-emerald-300"
                    : "text-muted-foreground"
                }`}
              >
                {tool.enabled ? "Active in Agent" : "Paused"}
              </Badge>
              {tool.requireApproval && (
                <Badge
                  variant="outline"
                  className="gap-1 border-amber-500/40 bg-amber-500/10 text-[11px] font-medium text-amber-700 dark:text-amber-400"
                >
                  <ShieldCheck className="size-3" /> Requires Approval
                </Badge>
              )}
            </div>
          </div>
          <DialogDescription className="mt-1 text-sm text-muted-foreground">
            {tool.description}
          </DialogDescription>

          {/* Subtabs */}
          <div className="mt-3 flex gap-1 border-b border-border/40 pt-1 -mb-4">
            <button
              type="button"
              onClick={() => setActiveTab("overview")}
              className={`border-b-2 px-3 py-1.5 text-xs font-medium transition-colors ${
                activeTab === "overview"
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              Overview & Parameters
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("schema")}
              className={`flex items-center gap-1.5 border-b-2 px-3 py-1.5 text-xs font-medium transition-colors ${
                activeTab === "schema"
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              <Code2 className="size-3.5" /> Agent Schema
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("curl")}
              className={`flex items-center gap-1.5 border-b-2 px-3 py-1.5 text-xs font-medium transition-colors ${
                activeTab === "curl"
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              <Terminal className="size-3.5" /> cURL Example
            </button>
          </div>
        </DialogHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          {activeTab === "overview" && (
            <div className="space-y-5">
              {/* Endpoint card */}
              <div className="rounded-xl border bg-muted/30 p-3.5">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-[11px] font-semibold tracking-wider text-muted-foreground uppercase">
                    HTTP Endpoint ({host})
                  </span>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => copyToClipboard(tool.urlTemplate, "url")}
                    className="h-6 w-6 text-muted-foreground hover:text-foreground"
                    title="Copy URL template"
                  >
                    {copiedKey === "url" ? <Check className="size-3 text-emerald-600" /> : <Copy className="size-3" />}
                  </Button>
                </div>
                <div className="mt-1.5 break-all font-mono text-xs leading-relaxed text-foreground">
                  {splitTemplate(tool.urlTemplate).map((seg, i) =>
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

              {/* Parameters list */}
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <h4 className="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
                    Parameters ({tool.params.length})
                  </h4>
                  <span className="text-[11px] text-muted-foreground">
                    Arguments extracted from chat
                  </span>
                </div>

                {tool.params.length === 0 ? (
                  <div className="rounded-lg border border-dashed p-4 text-center text-xs text-muted-foreground">
                    This tool takes no parameters; the model calls it with zero arguments.
                  </div>
                ) : (
                  <div className="overflow-hidden rounded-lg border">
                    <div className="grid grid-cols-[1.2fr_4.5rem_4.5rem_4rem_2fr] gap-2 border-b bg-muted/50 px-3 py-2 text-[11px] font-medium tracking-wider text-muted-foreground uppercase">
                      <span>Name</span>
                      <span>In</span>
                      <span>Type</span>
                      <span>Required</span>
                      <span>Description</span>
                    </div>
                    <div className="divide-y divide-border/60">
                      {tool.params.map((p, i) => (
                        <div
                          key={i}
                          className="grid grid-cols-[1.2fr_4.5rem_4.5rem_4rem_2fr] items-center gap-2 px-3 py-2 text-xs"
                        >
                          <span className="font-mono font-medium text-foreground">
                            {p.name}
                          </span>
                          <span>
                            <Badge
                              variant="outline"
                              className="font-mono text-[10px] px-1.5 py-0 capitalize"
                            >
                              {p.in}
                            </Badge>
                          </span>
                          <span className="font-mono text-muted-foreground">{p.type}</span>
                          <span>
                            {p.required ? (
                              <Badge className="bg-primary/10 text-primary font-normal text-[10px] px-1.5 py-0 border-0">
                                Yes
                              </Badge>
                            ) : (
                              <span className="text-muted-foreground text-[11px]">No</span>
                            )}
                          </span>
                          <span className="truncate text-muted-foreground" title={p.description}>
                            {p.description || "—"}
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>

              {/* Request Body if applicable */}
              {tool.bodyTemplate && (
                <div>
                  <h4 className="mb-2 text-xs font-semibold tracking-wider text-muted-foreground uppercase">
                    Body Template
                  </h4>
                  <pre className="overflow-x-auto rounded-lg border bg-muted/40 p-3 font-mono text-xs leading-relaxed text-foreground">
                    {tool.bodyTemplate}
                  </pre>
                </div>
              )}

              {/* Headers */}
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <h4 className="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
                    Request Headers ({Object.keys(tool.headers).length})
                  </h4>
                  <span className="text-[11px] text-muted-foreground">
                    Stored securely & masked
                  </span>
                </div>
                {Object.keys(tool.headers).length === 0 ? (
                  <p className="text-xs text-muted-foreground">No custom headers configured.</p>
                ) : (
                  <div className="space-y-1 rounded-lg border p-2.5">
                    {Object.entries(tool.headers).map(([k, v]) => (
                      <div key={k} className="flex items-center justify-between gap-2 font-mono text-xs">
                        <span className="font-medium text-foreground">{k}:</span>
                        <span className="text-muted-foreground">{v || HEADER_MASK}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Behavior & Meta */}
              <div className="rounded-xl border bg-card p-3.5 space-y-2.5">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-medium">Agent Approval Policy</span>
                  <span className="text-xs text-muted-foreground">
                    {tool.requireApproval
                      ? "Requires user approval each run"
                      : "Runs automatically without prompt"}
                  </span>
                </div>
                <div className="flex items-center justify-between border-t pt-2">
                  <span className="text-xs font-medium">Status</span>
                  <label className="flex items-center gap-2 cursor-pointer text-xs text-muted-foreground">
                    <span>{tool.enabled ? "Enabled" : "Disabled"}</span>
                    <Switch
                      checked={tool.enabled}
                      onCheckedChange={(checked) => onToggle(tool, checked)}
                      aria-label="Toggle tool active"
                    />
                  </label>
                </div>
              </div>
            </div>
          )}

          {activeTab === "schema" && (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <p className="text-xs text-muted-foreground">
                  The JSON Schema definition sent to the AI model during conversation:
                </p>
                <Button
                  variant="outline"
                  size="xs"
                  onClick={() => copyToClipboard(schemaJson, "schema")}
                  className="gap-1.5 text-xs"
                >
                  {copiedKey === "schema" ? <Check className="size-3 text-emerald-600" /> : <Copy className="size-3" />}
                  Copy JSON
                </Button>
              </div>
              <pre className="max-h-96 overflow-auto rounded-xl border bg-muted/40 p-4 font-mono text-xs leading-relaxed text-foreground">
                {schemaJson}
              </pre>
            </div>
          )}

          {activeTab === "curl" && (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <p className="text-xs text-muted-foreground">
                  Representative cURL command showing headers, method, and query parameters:
                </p>
                <Button
                  variant="outline"
                  size="xs"
                  onClick={() => copyToClipboard(curlCode, "curl")}
                  className="gap-1.5 text-xs"
                >
                  {copiedKey === "curl" ? <Check className="size-3 text-emerald-600" /> : <Copy className="size-3" />}
                  Copy cURL
                </Button>
              </div>
              <pre className="max-h-96 overflow-auto rounded-xl border bg-muted/40 p-4 font-mono text-xs leading-relaxed text-foreground">
                {curlCode}
              </pre>
            </div>
          )}
        </div>

        <DialogFooter className="border-t bg-muted/30 px-6 py-3 justify-between sm:justify-between">
          <div className="flex items-center gap-1.5">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onOpenChange(false);
                onDelete(tool);
              }}
              className="text-muted-foreground hover:text-destructive hover:bg-destructive/10 border-border"
            >
              <Trash2 className="size-3.5 mr-1" /> Delete
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onOpenChange(false);
                onDuplicate(tool);
              }}
              className="text-muted-foreground hover:text-foreground"
            >
              <Copy className="size-3.5 mr-1" /> Duplicate
            </Button>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
              Close
            </Button>
            <Button
              size="sm"
              onClick={() => {
                onOpenChange(false);
                onEdit(tool);
              }}
            >
              <Pencil className="size-3.5 mr-1" /> Edit Tool
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
