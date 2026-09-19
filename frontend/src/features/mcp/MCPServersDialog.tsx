import { useState } from "react";
import {
  CheckCircle2,
  ExternalLink,
  Layers,
  Loader2,
  Plug,
  Plus,
  Power,
  Trash2,
  Wrench,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import {
  useCreateMCPServer,
  useDeleteMCPServer,
  useMCPServers,
  useTestMCPServer,
  useToggleMCPServer,
  type MCPTestResult,
} from "./useMCPServers";

export interface MCPServersDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function MCPServersDialog({ open, onOpenChange }: MCPServersDialogProps) {
  const { data: servers, isLoading } = useMCPServers();
  const createServer = useCreateMCPServer();
  const toggleServer = useToggleMCPServer();
  const deleteServer = useDeleteMCPServer();
  const testServer = useTestMCPServer();

  const [isAdding, setIsAdding] = useState(false);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [appId, setAppId] = useState<string>("none");
  const [testResult, setTestResult] = useState<MCPTestResult | null>(null);

  async function handleTest() {
    if (!url.trim()) {
      toast.error("Please enter a server URL to test");
      return;
    }
    try {
      const res = await testServer.mutateAsync({ url: url.trim() });
      setTestResult(res);
      toast.success(`Connection successful! Discovered ${res.count} tool(s).`);
    } catch (err) {
      setTestResult(null);
      toast.error(err instanceof Error ? err.message : "Failed to connect to MCP server");
    }
  }

  function handleCreate() {
    if (!name.trim() || !url.trim()) {
      toast.error("Name and URL are required");
      return;
    }
    createServer.mutate(
      {
        name: name.trim().toLowerCase().replace(/\s+/g, "_"),
        url: url.trim(),
        appId: appId !== "none" ? appId : null,
      },
      {
        onSuccess: () => {
          setName("");
          setUrl("");
          setAppId("none");
          setTestResult(null);
          setIsAdding(false);
        },
      },
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl max-h-[85vh] flex flex-col p-6">
        <DialogHeader className="pb-3 border-b border-border/60">
          <div className="flex items-center gap-2.5">
            <div className="flex size-8 items-center justify-center rounded-xl bg-purple-600/10 text-purple-600 dark:text-purple-400">
              <Plug className="size-4" />
            </div>
            <div>
              <DialogTitle className="text-base font-semibold">
                Model Context Protocol (MCP) Servers
              </DialogTitle>
              <DialogDescription className="text-xs text-muted-foreground mt-0.5">
                Connect external MCP servers to expose tools to Herbie via standard JSON-RPC 2.0.
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <div className="flex items-center justify-between pt-2">
          <span className="text-xs font-semibold text-foreground">Registered MCP Servers</span>
          {!isAdding && (
            <Button size="xs" variant="outline" onClick={() => setIsAdding(true)} className="gap-1.5 text-xs">
              <Plus className="size-3" />
              <span>Add MCP Server</span>
            </Button>
          )}
        </div>

        {isAdding && (
          <div className="rounded-xl border border-border bg-muted/30 p-3.5 space-y-3 animate-in fade-in duration-150">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-foreground">Add New MCP Server</span>
              <Button size="icon-xs" variant="ghost" onClick={() => setIsAdding(false)}>
                ✕
              </Button>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <div className="space-y-1">
                <Label htmlFor="mcp-name" className="text-xs">
                  Server Name
                </Label>
                <Input
                  id="mcp-name"
                  placeholder="e.g. postgres_mcp"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="h-8 text-xs font-mono"
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="mcp-url" className="text-xs">
                  Endpoint URL
                </Label>
                <Input
                  id="mcp-url"
                  placeholder="http://localhost:3000/mcp"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  className="h-8 text-xs font-mono"
                />
              </div>
            </div>

            <div className="space-y-1">
              <Label className="text-xs">Link to App (Optional)</Label>
              <div className="flex flex-wrap gap-1.5 pt-0.5">
                {[
                  { id: "none", label: "General / Standalone" },
                  { id: "github", label: "GitHub (@github)" },
                  { id: "slack", label: "Slack (@slack)" },
                  { id: "google_calendar", label: "Google Calendar (@calendar)" },
                ].map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => setAppId(item.id)}
                    className={cn(
                      "px-2 py-1 text-[11px] rounded-md border font-medium transition-colors",
                      appId === item.id
                        ? "bg-purple-600/15 border-purple-500/40 text-purple-700 dark:text-purple-300 font-semibold"
                        : "border-border/70 text-muted-foreground hover:bg-muted/50 hover:text-foreground"
                    )}
                  >
                    {item.label}
                  </button>
                ))}
              </div>
            </div>

            {testResult && (
              <div className="rounded-lg border border-emerald-600/30 bg-emerald-500/10 p-2.5 text-xs text-emerald-800 dark:text-emerald-300">
                <div className="flex items-center gap-1.5 font-semibold">
                  <CheckCircle2 className="size-3.5 text-emerald-600" />
                  <span>Discovered {testResult.count} tool(s):</span>
                </div>
                <div className="flex flex-wrap gap-1.5 mt-1.5">
                  {testResult.tools.map((t) => (
                    <Badge key={t.name} variant="secondary" className="font-mono text-[10px] px-1.5 py-0">
                      {t.name}
                    </Badge>
                  ))}
                </div>
              </div>
            )}

            <div className="flex items-center justify-end gap-2 pt-1">
              <Button
                size="xs"
                variant="outline"
                type="button"
                disabled={testServer.isPending}
                onClick={handleTest}
                className="gap-1 text-xs"
              >
                {testServer.isPending ? <Loader2 className="size-3 animate-spin" /> : <Wrench className="size-3" />}
                <span>Test Connection</span>
              </Button>
              <Button
                size="xs"
                variant="default"
                disabled={createServer.isPending}
                onClick={handleCreate}
                className="text-xs"
              >
                {createServer.isPending ? "Saving…" : "Save Server"}
              </Button>
            </div>
          </div>
        )}

        <ScrollArea className="flex-1 max-h-72 pr-2">
          {isLoading ? (
            <p className="py-4 text-center text-xs text-muted-foreground">Loading MCP servers…</p>
          ) : !servers || servers.length === 0 ? (
            <div className="rounded-xl border border-dashed border-border/80 p-6 text-center text-xs space-y-1">
              <p className="font-medium text-foreground">No external MCP servers connected</p>
              <p className="text-muted-foreground text-[11px]">
                Herbie's built-in apps (Calendar, GitHub, Slack, Web Search) are active. Add custom MCP servers to expand capabilities.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {servers.map((srv) => (
                <div
                  key={srv.id}
                  className="flex items-center justify-between gap-3 rounded-xl border border-border bg-card p-3 text-xs shadow-2xs"
                >
                  <div className="flex items-center gap-2.5 min-w-0">
                    <div
                      className={`flex size-7 shrink-0 items-center justify-center rounded-lg ${
                        srv.enabled
                          ? "bg-emerald-600/10 text-emerald-600 dark:text-emerald-400"
                          : "bg-muted text-muted-foreground"
                      }`}
                    >
                      <Layers className="size-3.5" />
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-1.5">
                        <span className="font-semibold text-foreground truncate">{srv.name}</span>
                        <Badge
                          variant="outline"
                          className={
                            srv.enabled
                              ? "border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400 text-[10px] px-1.5 py-0 font-normal"
                              : "text-muted-foreground text-[10px] px-1.5 py-0 font-normal"
                          }
                        >
                          {srv.enabled ? "Active" : "Disabled"}
                        </Badge>
                        {srv.appId && (
                          <Badge variant="secondary" className="font-mono text-[10px] px-1.5 py-0 bg-purple-500/10 text-purple-700 dark:text-purple-300 border-purple-500/30">
                            Linked: @{srv.appId}
                          </Badge>
                        )}
                      </div>
                      <p className="font-mono text-[10px] text-muted-foreground truncate">{srv.url}</p>
                    </div>
                  </div>

                  <div className="flex items-center gap-1 shrink-0">
                    <Button
                      size="icon-xs"
                      variant="ghost"
                      title={srv.enabled ? "Disable server" : "Enable server"}
                      onClick={() => toggleServer.mutate(srv.id)}
                      className={srv.enabled ? "text-emerald-600 hover:text-emerald-700" : "text-muted-foreground"}
                    >
                      <Power className="size-3.5" />
                    </Button>
                    <Button
                      size="icon-xs"
                      variant="ghost"
                      title="Delete server"
                      onClick={() => deleteServer.mutate(srv.id)}
                      className="text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </ScrollArea>

        {/* MCP Server Capability info */}
        <div className="rounded-xl border border-border/60 bg-muted/30 p-2.5 text-[11px] text-muted-foreground flex items-center justify-between">
          <div className="flex items-center gap-1.5">
            <ExternalLink className="size-3 shrink-0" />
            <span>Herbie MCP Server endpoint: <code className="font-mono text-foreground font-semibold">POST /api/mcp</code></span>
          </div>
          <Badge variant="outline" className="text-[10px] font-normal">Active</Badge>
        </div>
      </DialogContent>
    </Dialog>
  );
}
