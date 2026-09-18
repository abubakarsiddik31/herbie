import { useState } from "react";
import { Copy, Trash2, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useTools } from "@/features/tools/useTools";
import { getNodeDefinition } from "./nodeTypes";
import type { NodeExecutionResult } from "@/lib/types";

interface NodeInspectorProps {
  node: {
    id: string;
    type: string;
    name: string;
    data: Record<string, unknown>;
  };
  executionResult?: NodeExecutionResult;
  webhookSlug?: string | null;
  onUpdate: (data: { name: string; config: Record<string, unknown> }) => void;
  onDelete: () => void;
  onClose: () => void;
}

export function NodeInspector({
  node,
  executionResult,
  webhookSlug,
  onUpdate,
  onDelete,
  onClose,
}: NodeInspectorProps) {
  const def = getNodeDefinition(node.type);
  const [activeTab, setActiveTab] = useState<"config" | "result">("config");

  const [name, setName] = useState(node.name || def.label);
  const [config, setConfig] = useState<Record<string, unknown>>({
    ...def.defaultData,
    ...node.data,
  });

  const { data: userTools } = useTools();

  function updateConfig(key: string, value: unknown) {
    const next = { ...config, [key]: value };
    setConfig(next);
    onUpdate({ name, config: next });
  }

  function handleNameChange(newName: string) {
    setName(newName);
    onUpdate({ name: newName, config });
  }

  const apiBase = window.location.origin;
  const webhookUrl = webhookSlug ? `${apiBase}/api/webhooks/${webhookSlug}` : "";

  function copyWebhookUrl() {
    if (!webhookUrl) return;
    navigator.clipboard.writeText(webhookUrl);
    toast.success("Webhook URL copied to clipboard");
  }

  return (
    <aside className="flex h-full w-84 md:w-96 flex-col border-l border-border bg-card shadow-xl z-20">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border/60 p-3.5">
        <div className="flex items-center gap-2">
          <div className="flex size-7 items-center justify-center rounded-md border border-border bg-muted">
            <def.icon className="size-4 text-primary" />
          </div>
          <div>
            <h3 className="text-xs font-semibold text-foreground tracking-tight">{def.label}</h3>
            <p className="text-[10px] text-muted-foreground font-mono">ID: {node.id}</p>
          </div>
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onDelete}
            className="text-muted-foreground hover:text-destructive"
            title="Delete node"
          >
            <Trash2 className="size-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onClose}
            className="text-muted-foreground hover:text-foreground"
          >
            <X className="size-4" />
          </Button>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex border-b border-border/60 px-3 pt-2 gap-2 text-xs">
        <button
          type="button"
          onClick={() => setActiveTab("config")}
          className={`pb-2 font-medium transition-colors border-b-2 ${
            activeTab === "config"
              ? "border-primary text-foreground"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          Parameters
        </button>
        <button
          type="button"
          onClick={() => setActiveTab("result")}
          className={`pb-2 font-medium transition-colors border-b-2 flex items-center gap-1.5 ${
            activeTab === "result"
              ? "border-primary text-foreground"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <span>Output Data</span>
          {executionResult && (
            <span
              className={`size-1.5 rounded-full ${
                executionResult.status === "success" ? "bg-emerald-500" : "bg-destructive"
              }`}
            />
          )}
        </button>
      </div>

      {/* Content */}
      <ScrollArea className="flex-1 p-4">
        {activeTab === "config" ? (
          <div className="space-y-4 text-xs">
            {/* Step Name */}
            <div className="space-y-1.5">
              <Label htmlFor="node-name" className="text-xs">Step Title</Label>
              <Input
                id="node-name"
                value={name}
                onChange={(e) => handleNameChange(e.target.value)}
                placeholder="e.g. Fetch Current Temperature"
                className="h-8 text-xs"
              />
            </div>

            {/* Node-specific configuration fields */}
            {node.type === "webhook" && (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">Incoming Webhook URL</Label>
                  <div className="flex gap-1.5">
                    <Input
                      readOnly
                      value={webhookUrl || "Activate webhook in workflow settings"}
                      className="h-8 font-mono text-[11px] bg-muted/50"
                    />
                    <Button
                      variant="outline"
                      size="icon-xs"
                      onClick={copyWebhookUrl}
                      disabled={!webhookUrl}
                      className="size-8 shrink-0"
                    >
                      <Copy className="size-3.5" />
                    </Button>
                  </div>
                  <p className="text-[11px] text-muted-foreground">
                    Accepts POST or GET requests. Headers, query params, and JSON body will flow into downstream steps as <code className="font-mono bg-muted px-1 rounded">{"{{ $json }}"}</code>.
                  </p>
                </div>
              </div>
            )}

            {node.type === "http_request" && (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">HTTP Method</Label>
                  <select
                    value={(config.method as string) || "GET"}
                    onChange={(e) => updateConfig("method", e.target.value)}
                    className="h-8 w-full rounded-md border border-input bg-background px-2.5 text-xs outline-none focus-visible:border-ring"
                  >
                    <option value="GET">GET</option>
                    <option value="POST">POST</option>
                    <option value="PUT">PUT</option>
                    <option value="PATCH">PATCH</option>
                    <option value="DELETE">DELETE</option>
                  </select>
                </div>

                <div className="space-y-1.5">
                  <Label className="text-xs">Request URL</Label>
                  <Input
                    value={(config.url as string) || ""}
                    onChange={(e) => updateConfig("url", e.target.value)}
                    placeholder="https://api.example.com/items/{{ $json.id }}"
                    className="h-8 font-mono text-xs"
                  />
                  <p className="text-[10px] text-muted-foreground">
                    Expressions like <code className="font-mono bg-muted px-1 rounded">{"{{ $json.key }}"}</code> or <code className="font-mono bg-muted px-1 rounded">{"{{ $credentials.name.token }}"}</code> are evaluated dynamically.
                  </p>
                </div>

                <div className="space-y-1.5">
                  <Label className="text-xs">Authentication</Label>
                  <select
                    value={((config.auth as { type?: string })?.type) || "none"}
                    onChange={(e) => {
                      const prev = (config.auth as Record<string, unknown>) || {};
                      updateConfig("auth", { ...prev, type: e.target.value });
                    }}
                    className="h-8 w-full rounded-md border border-input bg-background px-2.5 text-xs outline-none focus-visible:border-ring"
                  >
                    <option value="none">No Auth</option>
                    <option value="bearer">Bearer Token</option>
                    <option value="basic">Basic Auth</option>
                    <option value="api_key">API Key</option>
                  </select>
                </div>

                {((config.auth as { type?: string })?.type === "bearer") && (
                  <div className="space-y-1.5">
                    <Label className="text-xs">Bearer Token</Label>
                    <Input
                      value={((config.auth as { token?: string })?.token) || ""}
                      onChange={(e) => {
                        const prev = (config.auth as Record<string, unknown>) || {};
                        updateConfig("auth", { ...prev, token: e.target.value });
                      }}
                      placeholder="token or {{ $credentials.my_token.token }}"
                      className="h-8 font-mono text-xs"
                    />
                  </div>
                )}

                {((config.method as string) !== "GET" && (config.method as string) !== "DELETE") && (
                  <div className="space-y-1.5">
                    <Label className="text-xs">JSON Request Body</Label>
                    <textarea
                      value={
                        typeof config.body === "object"
                          ? JSON.stringify(config.body, null, 2)
                          : (config.body as string) || ""
                      }
                      onChange={(e) => updateConfig("body", e.target.value)}
                      placeholder={'{\n  "query": "{{ $json.text }}"\n}'}
                      rows={5}
                      className="w-full rounded-md border border-input bg-background p-2 font-mono text-xs outline-none focus-visible:border-ring"
                    />
                  </div>
                )}
              </div>
            )}

            {node.type === "golem_tool" && (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">Select Golem Tool</Label>
                  <select
                    value={(config.toolName as string) || ""}
                    onChange={(e) => updateConfig("toolName", e.target.value)}
                    className="h-8 w-full rounded-md border border-input bg-background px-2.5 text-xs outline-none focus-visible:border-ring"
                  >
                    <option value="">-- Choose custom tool --</option>
                    {userTools?.map((t) => (
                      <option key={t.id} value={t.name}>
                        {t.name} ({t.description.slice(0, 40)}...)
                      </option>
                    ))}
                  </select>
                </div>
              </div>
            )}

            {node.type === "llm_prompt" && (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">AI Model</Label>
                  <select
                    value={(config.model as string) || "gemini-2.5-flash"}
                    onChange={(e) => updateConfig("model", e.target.value)}
                    className="h-8 w-full rounded-md border border-input bg-background px-2.5 text-xs outline-none focus-visible:border-ring"
                  >
                    <option value="gemini-2.5-flash">Gemini 2.5 Flash</option>
                    <option value="gemini-2.5-pro">Gemini 2.5 Pro</option>
                    <option value="gpt-4o">GPT-4o</option>
                    <option value="claude-3-5-sonnet">Claude 3.5 Sonnet</option>
                  </select>
                </div>

                <div className="space-y-1.5">
                  <Label className="text-xs">System Prompt (optional)</Label>
                  <Input
                    value={(config.systemPrompt as string) || ""}
                    onChange={(e) => updateConfig("systemPrompt", e.target.value)}
                    placeholder="You are an expert concise assistant."
                    className="h-8 text-xs"
                  />
                </div>

                <div className="space-y-1.5">
                  <Label className="text-xs">User Prompt Template</Label>
                  <textarea
                    value={(config.prompt as string) || ""}
                    onChange={(e) => updateConfig("prompt", e.target.value)}
                    placeholder="Extract the sentiment from:\n{{ $json.text }}"
                    rows={4}
                    className="w-full rounded-md border border-input bg-background p-2 font-mono text-xs outline-none focus-visible:border-ring"
                  />
                </div>

                <div className="flex items-center justify-between pt-1">
                  <Label htmlFor="json-out" className="text-xs">Extract JSON structure</Label>
                  <Switch
                    id="json-out"
                    checked={Boolean(config.jsonOutput)}
                    onCheckedChange={(checked) => updateConfig("jsonOutput", checked)}
                  />
                </div>
              </div>
            )}

            {node.type === "condition" && (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">Expression Variable</Label>
                  <Input
                    value={(config.variable as string) || ""}
                    onChange={(e) => updateConfig("variable", e.target.value)}
                    placeholder="{{ $json.status }}"
                    className="h-8 font-mono text-xs"
                  />
                </div>

                <div className="space-y-1.5">
                  <Label className="text-xs">Comparison Operator</Label>
                  <select
                    value={(config.operator as string) || "equals"}
                    onChange={(e) => updateConfig("operator", e.target.value)}
                    className="h-8 w-full rounded-md border border-input bg-background px-2.5 text-xs outline-none focus-visible:border-ring"
                  >
                    <option value="equals">Equals (==)</option>
                    <option value="not_equals">Not Equals (!=)</option>
                    <option value="contains">Contains substring</option>
                    <option value="not_contains">Does not contain</option>
                    <option value="greater_than">Greater than (&gt;)</option>
                    <option value="less_than">Less than (&lt;)</option>
                    <option value="is_empty">Is Empty / Nil</option>
                    <option value="is_not_empty">Is Not Empty</option>
                  </select>
                </div>

                <div className="space-y-1.5">
                  <Label className="text-xs">Comparison Value</Label>
                  <Input
                    value={(config.value as string) || ""}
                    onChange={(e) => updateConfig("value", e.target.value)}
                    placeholder="success or 100"
                    className="h-8 font-mono text-xs"
                  />
                </div>
              </div>
            )}

            {node.type === "code_transform" && (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">Fields Definition (JSON mapping)</Label>
                  <textarea
                    value={
                      typeof config.fields === "object"
                        ? JSON.stringify(config.fields, null, 2)
                        : (config.fields as string) || ""
                    }
                    onChange={(e) => {
                      try {
                        const parsed = JSON.parse(e.target.value);
                        updateConfig("fields", parsed);
                      } catch {
                        // Keep text while typing
                      }
                    }}
                    placeholder={'{\n  "title": "{{ $json.body.issue.title }}",\n  "author": "{{ $json.body.sender.login }}"\n}'}
                    rows={6}
                    className="w-full rounded-md border border-input bg-background p-2 font-mono text-xs outline-none focus-visible:border-ring"
                  />
                  <p className="text-[10px] text-muted-foreground">
                    Constructs an output object with evaluated expressions for each field.
                  </p>
                </div>
              </div>
            )}

            {node.type === "slack" && (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">Slack Webhook URL</Label>
                  <Input
                    value={(config.webhookUrl as string) || ""}
                    onChange={(e) => updateConfig("webhookUrl", e.target.value)}
                    placeholder="https://hooks.slack.com/services/..."
                    className="h-8 font-mono text-xs"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-xs">Message Text</Label>
                  <textarea
                    value={(config.text as string) || ""}
                    onChange={(e) => updateConfig("text", e.target.value)}
                    placeholder="New alert: {{ $json.message }}"
                    rows={3}
                    className="w-full rounded-md border border-input bg-background p-2 text-xs outline-none focus-visible:border-ring"
                  />
                </div>
              </div>
            )}

            {node.type === "discord" && (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">Discord Webhook URL</Label>
                  <Input
                    value={(config.webhookUrl as string) || ""}
                    onChange={(e) => updateConfig("webhookUrl", e.target.value)}
                    placeholder="https://discord.com/api/webhooks/..."
                    className="h-8 font-mono text-xs"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-xs">Content</Label>
                  <textarea
                    value={(config.content as string) || ""}
                    onChange={(e) => updateConfig("content", e.target.value)}
                    placeholder="Discord update: {{ $json }}"
                    rows={3}
                    className="w-full rounded-md border border-input bg-background p-2 text-xs outline-none focus-visible:border-ring"
                  />
                </div>
              </div>
            )}

            {node.type === "webhook_response" && (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">Status Code</Label>
                  <Input
                    type="number"
                    value={Number(config.statusCode) || 200}
                    onChange={(e) => updateConfig("statusCode", Number(e.target.value))}
                    className="h-8 font-mono text-xs"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-xs">Response Body</Label>
                  <textarea
                    value={
                      typeof config.body === "object"
                        ? JSON.stringify(config.body, null, 2)
                        : (config.body as string) || ""
                    }
                    onChange={(e) => {
                      try {
                        const parsed = JSON.parse(e.target.value);
                        updateConfig("body", parsed);
                      } catch {
                        updateConfig("body", e.target.value);
                      }
                    }}
                    placeholder={'{\n  "ok": true,\n  "result": "{{ $json }}"\n}'}
                    rows={5}
                    className="w-full rounded-md border border-input bg-background p-2 font-mono text-xs outline-none focus-visible:border-ring"
                  />
                </div>
              </div>
            )}
          </div>
        ) : (
          /* Execution Result Tab */
          <div className="space-y-3 text-xs">
            {!executionResult ? (
              <div className="p-4 text-center text-muted-foreground">
                <p>No execution data for this node yet.</p>
                <p className="mt-1 text-[11px]">Click "Test Workflow" in the toolbar to run.</p>
              </div>
            ) : (
              <div className="space-y-3">
                <div className="flex items-center justify-between rounded-lg border border-border bg-muted/40 p-2.5">
                  <span className="font-medium text-foreground">Status</span>
                  <span
                    className={`font-semibold capitalize ${
                      executionResult.status === "success"
                        ? "text-emerald-500"
                        : executionResult.status === "running"
                        ? "text-blue-500"
                        : "text-destructive"
                    }`}
                  >
                    {executionResult.status} ({executionResult.durationMs}ms)
                  </span>
                </div>

                {executionResult.error && (
                  <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-2.5 text-destructive text-[11px]">
                    <span className="font-semibold">Error:</span> {executionResult.error}
                  </div>
                )}

                <div className="space-y-1.5">
                  <Label className="text-xs text-muted-foreground">Node Output Payload</Label>
                  <pre className="max-h-60 overflow-auto rounded-lg border border-border bg-muted/50 p-2 font-mono text-[11px] leading-relaxed">
                    {JSON.stringify(executionResult.output ?? null, null, 2)}
                  </pre>
                </div>

                {executionResult.input !== undefined && (
                  <div className="space-y-1.5 pt-2 border-t border-border/40">
                    <Label className="text-xs text-muted-foreground">Node Input Payload</Label>
                    <pre className="max-h-40 overflow-auto rounded-lg border border-border bg-muted/50 p-2 font-mono text-[11px] leading-relaxed">
                      {JSON.stringify(executionResult.input ?? null, null, 2)}
                    </pre>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </ScrollArea>
    </aside>
  );
}
