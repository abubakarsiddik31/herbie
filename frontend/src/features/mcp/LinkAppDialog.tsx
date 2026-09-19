import { useState, useEffect } from "react";
import {
  Calendar,
  CheckCircle2,
  ExternalLink,
  GitBranch,
  Key,
  Loader2,
  MessageSquare,
  Plug,
  RefreshCw,
  ShieldAlert,
  Sparkles,
  Trash2,
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
import {
  useCreateMCPServer,
  useMCPServers,
  useTestMCPServer,
  useUnlinkAppMCPServer,
  type MCPTestResult,
} from "./useMCPServers";
import { useDisconnectToolOAuth, useToolOAuthProviders } from "@/features/workflows/useWorkflows";
import { apiFetch } from "@/lib/api";
import { cn } from "@/lib/utils";

export interface LinkAppDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialAppId?: "google_calendar" | "github" | "slack" | string;
}

interface AppPreset {
  id: string;
  name: string;
  icon: "calendar" | "github" | "slack";
  color: string;
  defaultServerName: string;
  defaultUrl: string;
  authHeaderKey: string;
  authHeaderPlaceholder: string;
  instructions: string;
  sampleTools: string[];
}

const APP_PRESETS: Record<string, AppPreset> = {
  github: {
    id: "github",
    name: "GitHub",
    icon: "github",
    color: "text-neutral-900 dark:text-neutral-100",
    defaultServerName: "GitHub MCP Server",
    defaultUrl: "http://localhost:8000/mcp",
    authHeaderKey: "Authorization",
    authHeaderPlaceholder: "Bearer ghp_...",
    instructions: "Run a GitHub MCP server (e.g. via @modelcontextprotocol/server-github HTTP bridge) with a personal access token.",
    sampleTools: ["list_issues", "create_issue", "list_pull_requests", "get_file_contents"],
  },
  slack: {
    id: "slack",
    name: "Slack",
    icon: "slack",
    color: "text-emerald-600 dark:text-emerald-400",
    defaultServerName: "Slack MCP Server",
    defaultUrl: "http://localhost:8001/mcp",
    authHeaderKey: "Authorization",
    authHeaderPlaceholder: "Bearer xoxb-...",
    instructions: "Run a Slack MCP server with your Slack Bot OAuth token (xoxb-...).",
    sampleTools: ["post_message", "list_channels", "get_thread", "reply_to_thread"],
  },
  google_calendar: {
    id: "google_calendar",
    name: "Google Calendar",
    icon: "calendar",
    color: "text-sky-600 dark:text-sky-400",
    defaultServerName: "Google Calendar MCP",
    defaultUrl: "http://localhost:8002/mcp",
    authHeaderKey: "Authorization",
    authHeaderPlaceholder: "Bearer ya29....",
    instructions: "Run a Google Calendar or Google Workspace MCP server.",
    sampleTools: ["list_events", "create_event", "delete_event", "search_agenda"],
  },
};

export function LinkAppDialog({ open, onOpenChange, initialAppId = "github" }: LinkAppDialogProps) {
  const [selectedApp, setSelectedApp] = useState<string>(initialAppId);
  const [tab, setTab] = useState<"mcp" | "oauth">("mcp");

  // Form states
  const [serverName, setServerName] = useState("");
  const [serverUrl, setUrl] = useState("");
  const [authHeader, setAuthHeader] = useState("");
  const [testResult, setTestResult] = useState<MCPTestResult | null>(null);
  const [oauthConnecting, setOauthConnecting] = useState(false);

  const { data: oauthProviders } = useToolOAuthProviders();
  const { data: mcpServers } = useMCPServers();
  const createServer = useCreateMCPServer();
  const unlinkApp = useUnlinkAppMCPServer();
  const disconnectOAuth = useDisconnectToolOAuth();
  const testServer = useTestMCPServer();

  function resetForm(appId: string) {
    const p = APP_PRESETS[appId];
    if (p) {
      setServerName(p.defaultServerName);
      setUrl(p.defaultUrl);
      setAuthHeader("");
      setTestResult(null);
    }
  }

  // Keep selected app synced with initialAppId when dialog opens
  useEffect(() => {
    if (open) {
      const valid = APP_PRESETS[initialAppId] ? initialAppId : "github";
      setSelectedApp(valid);
      resetForm(valid);
    }
  }, [open, initialAppId]);

  function handleSelectApp(appId: string) {
    setSelectedApp(appId);
    resetForm(appId);
  }

  const preset = APP_PRESETS[selectedApp] || APP_PRESETS.github;
  const currentProvider = oauthProviders?.find((p) => p.id === selectedApp);
  const linkedMcpServer = mcpServers?.find((s) => s.appId === selectedApp && s.enabled);

  const isConnected = !!currentProvider?.connected || !!linkedMcpServer;
  const connectedVia = linkedMcpServer ? (currentProvider?.connected ? "both" : "mcp") : (currentProvider?.connectedVia || "oauth");

  async function handleTest() {
    if (!serverUrl.trim()) {
      toast.error("Please provide an MCP server endpoint URL");
      return;
    }

    const headers: Record<string, string> = {};
    if (authHeader.trim()) {
      headers[preset.authHeaderKey] = authHeader.trim();
    }

    try {
      const res = await testServer.mutateAsync({
        url: serverUrl.trim(),
        headers: Object.keys(headers).length > 0 ? headers : undefined,
      });
      setTestResult(res);
      toast.success(`MCP connection verified! Discovered ${res.count} tool(s).`);
    } catch (err) {
      setTestResult(null);
      toast.error(err instanceof Error ? err.message : "Failed to reach MCP server");
    }
  }

  async function handleSaveMcp() {
    if (!serverName.trim() || !serverUrl.trim()) {
      toast.error("Name and URL are required");
      return;
    }

    const headers: Record<string, string> = {};
    if (authHeader.trim()) {
      headers[preset.authHeaderKey] = authHeader.trim();
    }

    createServer.mutate(
      {
        name: serverName.trim().toLowerCase().replace(/\s+/g, "_"),
        url: serverUrl.trim(),
        appId: selectedApp,
        headers: Object.keys(headers).length > 0 ? headers : undefined,
      },
      {
        onSuccess: () => {
          toast.success(`Linked ${preset.name} to MCP server successfully!`);
          onOpenChange(false);
        },
      },
    );
  }

  async function handleOAuthConnect() {
    setOauthConnecting(true);
    try {
      const dest = window.location.pathname + window.location.search;
      const data = await apiFetch<{ url: string }>(
        `/api/tool-oauth/${selectedApp}/start?return_to=${encodeURIComponent(dest)}`,
      );
      if (data.url) {
        window.location.href = data.url;
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to start ${preset.name} OAuth`);
      setOauthConnecting(false);
    }
  }

  async function handleDisconnect() {
    if (linkedMcpServer) {
      await unlinkApp.mutateAsync(linkedMcpServer.id);
    }
    if (currentProvider?.connected) {
      await disconnectOAuth.mutateAsync(selectedApp);
    }
    toast.success(`Disconnected ${preset.name}`);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] flex flex-col p-6 gap-0">
        <DialogHeader className="pb-4 border-b border-border/60">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className={cn("flex size-9 items-center justify-center rounded-xl bg-muted/80 border border-border/70", preset.color)}>
                {preset.icon === "calendar" && <Calendar className="size-5" />}
                {preset.icon === "github" && <GitBranch className="size-5" />}
                {preset.icon === "slack" && <MessageSquare className="size-5" />}
              </div>
              <div>
                <DialogTitle className="text-base font-semibold">
                  Link {preset.name} App
                </DialogTitle>
                <DialogDescription className="text-xs text-muted-foreground mt-0.5">
                  Link with an external Model Context Protocol (MCP) server or direct OAuth.
                </DialogDescription>
              </div>
            </div>

            {/* App switcher pills */}
            <div className="flex items-center bg-muted/60 p-1 rounded-lg border border-border/60 gap-1">
              {Object.values(APP_PRESETS).map((p) => {
                const active = selectedApp === p.id;
                return (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => handleSelectApp(p.id)}
                    className={cn(
                      "px-2.5 py-1 text-xs font-medium rounded-md transition-colors",
                      active
                        ? "bg-background text-foreground shadow-xs font-semibold"
                        : "text-muted-foreground hover:text-foreground"
                    )}
                  >
                    {p.name}
                  </button>
                );
              })}
            </div>
          </div>
        </DialogHeader>

        <ScrollArea className="flex-1 overflow-y-auto px-1 py-4">
          <div className="space-y-5">
            {/* Status Card if connected */}
            {isConnected && (
              <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-4 space-y-3">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2.5">
                    <CheckCircle2 className="size-5 text-emerald-500 shrink-0" />
                    <div>
                      <h4 className="text-xs font-semibold text-emerald-950 dark:text-emerald-100 flex items-center gap-2">
                        {preset.name} is Connected
                        <Badge variant="outline" className="text-[10px] border-emerald-500/40 text-emerald-700 dark:text-emerald-300 font-mono">
                          {connectedVia === "mcp" ? "Via MCP Server" : connectedVia === "both" ? "OAuth + MCP" : "Via OAuth"}
                        </Badge>
                      </h4>
                      <p className="text-[11px] text-muted-foreground mt-0.5">
                        {linkedMcpServer
                          ? `Powered by MCP Server "${linkedMcpServer.name}" (${linkedMcpServer.url}).`
                          : `Connected via OAuth credential.`}
                      </p>
                    </div>
                  </div>

                  <Button
                    size="xs"
                    variant="outline"
                    onClick={handleDisconnect}
                    disabled={unlinkApp.isPending || disconnectOAuth.isPending}
                    className="text-xs text-destructive hover:bg-destructive/10 hover:text-destructive border-destructive/30 gap-1.5"
                  >
                    {(unlinkApp.isPending || disconnectOAuth.isPending) ? (
                      <Loader2 className="size-3 animate-spin" />
                    ) : (
                      <Trash2 className="size-3" />
                    )}
                    <span>Unlink / Disconnect</span>
                  </Button>
                </div>
              </div>
            )}

            {/* Connection Mode Selection Tabs */}
            <div className="flex items-center gap-2 border-b border-border/60 pb-2">
              <button
                type="button"
                onClick={() => setTab("mcp")}
                className={cn(
                  "flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors",
                  tab === "mcp"
                    ? "bg-purple-600/15 text-purple-700 dark:text-purple-300 font-semibold border border-purple-500/30"
                    : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
                )}
              >
                <Plug className="size-3.5" />
                <span>Link via MCP Server (Recommended)</span>
              </button>

              <button
                type="button"
                onClick={() => setTab("oauth")}
                className={cn(
                  "flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors",
                  tab === "oauth"
                    ? "bg-sky-600/15 text-sky-700 dark:text-sky-300 font-semibold border border-sky-500/30"
                    : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
                )}
              >
                <Key className="size-3.5" />
                <span>Connect via OAuth</span>
              </button>
            </div>

            {/* MCP Linking Tab */}
            {tab === "mcp" && (
              <div className="space-y-4">
                <div className="rounded-xl border border-border/80 bg-muted/30 p-3 text-xs space-y-1.5">
                  <div className="flex items-center gap-1.5 font-medium text-foreground">
                    <Sparkles className="size-3.5 text-purple-500" />
                    <span>Model Context Protocol Integration for {preset.name}</span>
                  </div>
                  <p className="text-[11px] text-muted-foreground leading-relaxed">
                    {preset.instructions} Discovered tools from this server will be automatically available to Herbie when you mention @{selectedApp === "google_calendar" ? "calendar" : selectedApp}.
                  </p>
                  <div className="flex flex-wrap gap-1.5 pt-1">
                    {preset.sampleTools.map((t) => (
                      <Badge key={t} variant="secondary" className="text-[10px] font-mono px-1.5 py-0">
                        {t}
                      </Badge>
                    ))}
                  </div>
                </div>

                <div className="space-y-3">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <div className="space-y-1">
                      <Label htmlFor="mcp-name" className="text-xs">
                        Server Name
                      </Label>
                      <Input
                        id="mcp-name"
                        value={serverName}
                        onChange={(e) => setServerName(e.target.value)}
                        placeholder="e.g. github_mcp"
                        className="h-8 text-xs font-mono"
                      />
                    </div>

                    <div className="space-y-1">
                      <Label htmlFor="mcp-url" className="text-xs">
                        MCP Endpoint URL
                      </Label>
                      <Input
                        id="mcp-url"
                        value={serverUrl}
                        onChange={(e) => setUrl(e.target.value)}
                        placeholder="http://localhost:8000/mcp"
                        className="h-8 text-xs font-mono"
                      />
                    </div>
                  </div>

                  <div className="space-y-1">
                    <Label htmlFor="mcp-auth" className="text-xs flex items-center justify-between">
                      <span>{preset.authHeaderKey} Header (Optional)</span>
                      <span className="text-[10px] text-muted-foreground">Passed on every tool call</span>
                    </Label>
                    <Input
                      id="mcp-auth"
                      type="password"
                      value={authHeader}
                      onChange={(e) => setAuthHeader(e.target.value)}
                      placeholder={preset.authHeaderPlaceholder}
                      className="h-8 text-xs font-mono"
                    />
                  </div>
                </div>

                {/* Test Result Display */}
                {testResult && (
                  <div className="rounded-xl border border-emerald-500/40 bg-emerald-500/10 p-3 space-y-2">
                    <div className="flex items-center gap-2">
                      <CheckCircle2 className="size-4 text-emerald-500" />
                      <span className="text-xs font-medium text-emerald-950 dark:text-emerald-100">
                        Endpoint Active — Discovered {testResult.count} Tool(s)
                      </span>
                    </div>
                    <div className="flex flex-wrap gap-1.5">
                      {testResult.tools.map((t) => (
                        <div
                          key={t.name}
                          className="rounded-md border border-emerald-500/30 bg-background/80 px-2 py-1 text-[10px] font-mono text-foreground"
                          title={t.description}
                        >
                          <span className="font-semibold text-emerald-600 dark:text-emerald-400">
                            {selectedApp}_{t.name}
                          </span>
                          {t.description && (
                            <span className="text-muted-foreground ml-1 font-sans truncate max-w-[180px] inline-block align-bottom">
                              — {t.description}
                            </span>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                <div className="flex items-center justify-end gap-2 pt-2 border-t border-border/50">
                  <Button
                    size="sm"
                    type="button"
                    variant="outline"
                    onClick={handleTest}
                    disabled={testServer.isPending || !serverUrl.trim()}
                    className="text-xs gap-1.5"
                  >
                    {testServer.isPending ? (
                      <Loader2 className="size-3.5 animate-spin" />
                    ) : (
                      <RefreshCw className="size-3.5" />
                    )}
                    <span>Test Connection</span>
                  </Button>

                  <Button
                    size="sm"
                    type="button"
                    onClick={handleSaveMcp}
                    disabled={createServer.isPending || !serverName.trim() || !serverUrl.trim()}
                    className="text-xs bg-purple-600 hover:bg-purple-700 text-white gap-1.5 shadow-xs"
                  >
                    {createServer.isPending ? (
                      <Loader2 className="size-3.5 animate-spin" />
                    ) : (
                      <Plug className="size-3.5" />
                    )}
                    <span>Link {preset.name} via MCP</span>
                  </Button>
                </div>
              </div>
            )}

            {/* OAuth Tab */}
            {tab === "oauth" && (
              <div className="space-y-4">
                <div className="rounded-xl border border-border/80 bg-muted/30 p-3.5 space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-semibold text-foreground">
                      Direct {preset.name} OAuth Authentication
                    </span>
                    <Badge
                      variant="outline"
                      className={cn(
                        "text-[10px]",
                        currentProvider?.configured
                          ? "border-emerald-500/40 text-emerald-600 dark:text-emerald-400"
                          : "border-amber-500/40 text-amber-600 dark:text-amber-400"
                      )}
                    >
                      {currentProvider?.configured ? "OAuth Configured" : "Config Required in .env"}
                    </Badge>
                  </div>
                  <p className="text-[11px] text-muted-foreground leading-relaxed">
                    Authenticate directly through your browser with {preset.name} credentials.
                  </p>
                  {currentProvider?.scopes && currentProvider.scopes.length > 0 && (
                    <div className="flex items-center gap-1.5 pt-1">
                      <span className="text-[10px] text-muted-foreground">Requested scopes:</span>
                      {currentProvider.scopes.map((s) => (
                        <Badge key={s} variant="secondary" className="text-[10px] font-mono px-1 py-0">
                          {s}
                        </Badge>
                      ))}
                    </div>
                  )}
                </div>

                {!currentProvider?.configured ? (
                  <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-3.5 space-y-2">
                    <div className="flex items-center gap-2 text-xs font-medium text-amber-900 dark:text-amber-200">
                      <ShieldAlert className="size-4 text-amber-600 shrink-0" />
                      <span>OAuth not configured in server environment</span>
                    </div>
                    <p className="text-[11px] text-muted-foreground leading-relaxed">
                      To connect via OAuth, configure the client credentials in your server environment (e.g. <code className="font-mono text-xs">OAUTH_{preset.name.toUpperCase().replace(/\s+/g, "_")}_CLIENT_ID</code>). Alternatively, you can use the <strong>Link via MCP Server</strong> tab above to connect immediately without server restarts!
                    </p>
                  </div>
                ) : (
                  <div className="flex items-center justify-end pt-2 border-t border-border/50">
                    <Button
                      size="sm"
                      onClick={handleOAuthConnect}
                      disabled={oauthConnecting}
                      className="text-xs gap-1.5"
                    >
                      {oauthConnecting ? (
                        <Loader2 className="size-3.5 animate-spin" />
                      ) : (
                        <ExternalLink className="size-3.5" />
                      )}
                      <span>Authorize {preset.name}</span>
                    </Button>
                  </div>
                )}
              </div>
            )}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
