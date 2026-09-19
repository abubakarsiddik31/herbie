import { useState } from "react";
import {
  Calendar,
  CheckCircle2,
  ChevronDown,
  ExternalLink,
  GitBranch,
  Globe,
  Key,
  Layers,
  Loader2,
  MessageSquare,
  Plug,
  Sparkles,
  Terminal,
  Trash2,
  Wrench,
} from "lucide-react";
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
  useLinkCatalogApp,
  useMCPCatalog,
  useUnlinkCatalogApp,
  type CatalogApp,
} from "./useMCPCatalog";
import { useCreateMCPServer, useTestMCPServer, type MCPTestResult } from "./useMCPServers";
import { useToolOAuthProviders } from "@/features/workflows/useWorkflows";
import { apiFetch } from "@/lib/api";
import { cn } from "@/lib/utils";
import { toast } from "sonner";

export interface LinkAppDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialAppId?: string;
}

export function LinkAppDialog({ open, onOpenChange }: LinkAppDialogProps) {
  const [filterCategory, setFilterCategory] = useState<string>("all");
  const [tokenInputs, setTokenInputs] = useState<Record<string, string>>({});
  const [showTokenFor, setShowTokenFor] = useState<string | null>(null);

  // Advanced custom server form states
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [customName, setCustomName] = useState("");
  const [customUrl, setCustomUrl] = useState("");
  const [customTestResult, setCustomTestResult] = useState<MCPTestResult | null>(null);

  const { data: catalog, isLoading } = useMCPCatalog();
  const { data: oauthProviders } = useToolOAuthProviders();
  const linkApp = useLinkCatalogApp();
  const unlinkApp = useUnlinkCatalogApp();

  const createServer = useCreateMCPServer();
  const testServer = useTestMCPServer();

  const categories = ["all", "Developer", "Communication", "Productivity", "Web & Data"];

  const filteredApps = (catalog || []).filter((app) => {
    if (filterCategory === "all") return true;
    return app.category.toLowerCase() === filterCategory.toLowerCase();
  });

  async function handleOneClickLink(app: CatalogApp) {
    const token = tokenInputs[app.id]?.trim();
    await linkApp.mutateAsync({ appId: app.id, token: token || undefined });
    setShowTokenFor(null);
  }

  async function handleOneClickUnlink(app: CatalogApp) {
    await unlinkApp.mutateAsync(app.id);
  }

  async function handleOAuthStart(appId: string) {
    try {
      const dest = window.location.pathname + window.location.search;
      const data = await apiFetch<{ url: string }>(
        `/api/tool-oauth/${appId}/start?return_to=${encodeURIComponent(dest)}`,
      );
      if (data.url) {
        window.location.href = data.url;
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to start ${appId} OAuth`);
    }
  }

  async function handleTestCustom() {
    if (!customUrl.trim()) {
      toast.error("Please enter a custom MCP URL");
      return;
    }
    try {
      const res = await testServer.mutateAsync({ url: customUrl.trim() });
      setCustomTestResult(res);
      toast.success(`Discovered ${res.count} tool(s)!`);
    } catch (err) {
      setCustomTestResult(null);
      toast.error(err instanceof Error ? err.message : "Failed to connect to MCP server");
    }
  }

  function handleCreateCustom() {
    if (!customName.trim() || !customUrl.trim()) {
      toast.error("Name and URL are required");
      return;
    }
    createServer.mutate(
      {
        name: customName.trim().toLowerCase().replace(/\s+/g, "_"),
        url: customUrl.trim(),
      },
      {
        onSuccess: () => {
          setCustomName("");
          setCustomUrl("");
          setCustomTestResult(null);
          setShowAdvanced(false);
        },
      },
    );
  }

  function getAppIcon(icon: string) {
    switch (icon) {
      case "github":
        return <GitBranch className="size-4 text-neutral-800 dark:text-neutral-200" />;
      case "slack":
        return <MessageSquare className="size-4 text-emerald-500" />;
      case "calendar":
        return <Calendar className="size-4 text-sky-500" />;
      case "globe":
        return <Globe className="size-4 text-cyan-500" />;
      case "terminal":
        return <Terminal className="size-4 text-amber-500" />;
      default:
        return <Plug className="size-4 text-purple-500" />;
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] flex flex-col p-6 gap-0">
        <DialogHeader className="pb-3 border-b border-border/60">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2.5">
              <div className="flex size-8 items-center justify-center rounded-xl bg-purple-600/10 text-purple-600 dark:text-purple-400">
                <Sparkles className="size-4" />
              </div>
              <div>
                <DialogTitle className="text-base font-semibold">
                  Apps & MCP Integrations
                </DialogTitle>
                <DialogDescription className="text-xs text-muted-foreground mt-0.5">
                  1-Click link curated Model Context Protocol apps to your AI agent.
                </DialogDescription>
              </div>
            </div>

            <Badge variant="outline" className="text-[10px] border-purple-500/30 text-purple-600 dark:text-purple-400 gap-1 font-mono">
              <Plug className="size-2.5" />
              <span>Zero Configuration</span>
            </Badge>
          </div>

          {/* Category Filter Pills */}
          <div className="flex items-center gap-1.5 pt-3 overflow-x-auto pb-1">
            {categories.map((cat) => {
              const active = filterCategory === cat;
              return (
                <button
                  key={cat}
                  type="button"
                  onClick={() => setFilterCategory(cat)}
                  className={cn(
                    "px-2.5 py-1 text-xs font-medium rounded-lg transition-colors capitalize shrink-0",
                    active
                      ? "bg-purple-600/15 border border-purple-500/40 text-purple-700 dark:text-purple-300 font-semibold"
                      : "text-muted-foreground hover:bg-muted/50 hover:text-foreground border border-transparent"
                  )}
                >
                  {cat === "all" ? "All Apps" : cat}
                </button>
              );
            })}
          </div>
        </DialogHeader>

        <ScrollArea className="flex-1 overflow-y-auto px-1 py-4">
          <div className="space-y-3">
            {isLoading ? (
              <p className="text-xs text-muted-foreground text-center py-8">Loading apps catalog…</p>
            ) : filteredApps.length === 0 ? (
              <p className="text-xs text-muted-foreground text-center py-8">No apps found in this category.</p>
            ) : (
              filteredApps.map((app) => {
                const isLinked = app.connected;
                const isLinking = linkApp.isPending && linkApp.variables?.appId === app.id;
                const isUnlinking = unlinkApp.isPending && unlinkApp.variables === app.id;
                const isTokenOpen = showTokenFor === app.id;
                const oauthProvider = oauthProviders?.find((p) => p.id === app.id);

                return (
                  <div
                    key={app.id}
                    className={cn(
                      "rounded-xl border p-3.5 transition-all shadow-2xs space-y-2.5",
                      isLinked
                        ? "border-emerald-500/40 bg-emerald-500/5 dark:bg-emerald-500/10"
                        : "border-border/80 bg-card hover:border-border"
                    )}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex items-start gap-3 min-w-0">
                        <div className="flex size-9 shrink-0 items-center justify-center rounded-xl border border-border/80 bg-muted/60 mt-0.5">
                          {getAppIcon(app.icon)}
                        </div>

                        <div className="min-w-0 space-y-1">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className="font-semibold text-xs text-foreground">{app.name}</span>
                            <span className="font-mono text-[10px] text-muted-foreground">@{app.mention}</span>
                            <Badge variant="secondary" className="text-[9px] px-1 py-0 h-3.5 font-normal">
                              {app.category}
                            </Badge>
                            {isLinked && (
                              <Badge variant="outline" className="border-emerald-600/40 text-emerald-700 dark:text-emerald-300 text-[9px] gap-1 px-1.5 py-0 font-normal">
                                <CheckCircle2 className="size-2.5" />
                                <span>Connected</span>
                              </Badge>
                            )}
                          </div>

                          <p className="text-[11px] text-muted-foreground leading-relaxed">
                            {app.description}
                          </p>

                          {/* Capabilities/Tools Pills */}
                          <div className="flex flex-wrap gap-1 pt-1">
                            {app.tools.map((t) => (
                              <span
                                key={t.name}
                                className="inline-block rounded px-1.5 py-0.5 text-[9px] font-mono bg-muted/70 text-muted-foreground"
                                title={t.description}
                              >
                                {t.name}
                              </span>
                            ))}
                          </div>
                        </div>
                      </div>

                      {/* 1-Click Action Buttons */}
                      <div className="flex items-center gap-1.5 shrink-0 pt-0.5">
                        {isLinked ? (
                          <Button
                            size="xs"
                            variant="outline"
                            onClick={() => handleOneClickUnlink(app)}
                            disabled={isUnlinking}
                            className="h-7 text-xs text-destructive hover:bg-destructive/10 border-destructive/30 gap-1"
                          >
                            {isUnlinking ? (
                              <Loader2 className="size-3 animate-spin" />
                            ) : (
                              <Trash2 className="size-3" />
                            )}
                            <span>Unlink</span>
                          </Button>
                        ) : (
                          <div className="flex items-center gap-1.5">
                            {app.authType === "token" && !isTokenOpen && (
                              <button
                                type="button"
                                onClick={() => setShowTokenFor(app.id)}
                                className="text-[10px] text-muted-foreground hover:text-foreground underline underline-offset-2"
                              >
                                Add Token
                              </button>
                            )}

                            {app.authType === "oauth" && oauthProvider?.configured && (
                              <Button
                                size="xs"
                                variant="outline"
                                onClick={() => handleOAuthStart(app.id)}
                                className="h-7 text-xs gap-1 text-sky-600 dark:text-sky-400 border-sky-500/30 hover:bg-sky-500/10"
                              >
                                <ExternalLink className="size-3" />
                                <span>OAuth</span>
                              </Button>
                            )}

                            <Button
                              size="xs"
                              variant="default"
                              onClick={() => handleOneClickLink(app)}
                              disabled={isLinking}
                              className="h-7 text-xs font-medium bg-purple-600 hover:bg-purple-700 text-white gap-1.5 shadow-xs"
                            >
                              {isLinking ? (
                                <Loader2 className="size-3 animate-spin" />
                              ) : (
                                <Sparkles className="size-3" />
                              )}
                              <span>1-Click Link</span>
                            </Button>
                          </div>
                        )}
                      </div>
                    </div>

                    {/* Optional Token input accordion */}
                    {isTokenOpen && !isLinked && (
                      <div className="pt-2 border-t border-border/50 flex items-center gap-2">
                        <div className="relative flex-1">
                          <Key className="size-3.5 text-muted-foreground absolute left-2.5 top-2.5" />
                          <Input
                            type="password"
                            placeholder={app.authPrompt || "Paste optional access token..."}
                            value={tokenInputs[app.id] || ""}
                            onChange={(e) =>
                              setTokenInputs((prev) => ({ ...prev, [app.id]: e.target.value }))
                            }
                            className="h-8 pl-8 text-xs font-mono"
                          />
                        </div>
                        <Button
                          size="xs"
                          onClick={() => handleOneClickLink(app)}
                          disabled={isLinking}
                          className="h-8 text-xs bg-purple-600 hover:bg-purple-700 text-white shrink-0"
                        >
                          Save & Link
                        </Button>
                        <Button
                          size="xs"
                          variant="ghost"
                          onClick={() => setShowTokenFor(null)}
                          className="h-8 text-xs text-muted-foreground"
                        >
                          Cancel
                        </Button>
                      </div>
                    )}
                  </div>
                );
              })
            )}

            {/* Advanced External MCP Server Collapsible for Power Users */}
            <div className="pt-3 border-t border-border/60">
              <button
                type="button"
                onClick={() => setShowAdvanced(!showAdvanced)}
                className="flex w-full items-center justify-between text-xs text-muted-foreground hover:text-foreground font-medium py-1"
              >
                <div className="flex items-center gap-1.5">
                  <Layers className="size-3.5 text-muted-foreground" />
                  <span>Developer: Connect Custom External MCP Endpoint</span>
                </div>
                <ChevronDown className={cn("size-3.5 transition-transform", showAdvanced && "rotate-180")} />
              </button>

              {showAdvanced && (
                <div className="mt-2 rounded-xl border border-border/80 bg-muted/20 p-3 space-y-3">
                  <p className="text-[11px] text-muted-foreground">
                    Connect an external Model Context Protocol server running locally or on a private network.
                  </p>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                    <div className="space-y-1">
                      <Label className="text-xs">Server Name</Label>
                      <Input
                        value={customName}
                        onChange={(e) => setCustomName(e.target.value)}
                        placeholder="e.g. postgres_mcp"
                        className="h-8 text-xs font-mono"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label className="text-xs">Server URL</Label>
                      <Input
                        value={customUrl}
                        onChange={(e) => setCustomUrl(e.target.value)}
                        placeholder="http://localhost:3000/mcp"
                        className="h-8 text-xs font-mono"
                      />
                    </div>
                  </div>

                  {customTestResult && (
                    <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-2 text-xs text-emerald-800 dark:text-emerald-300">
                      Discovered {customTestResult.count} tool(s): {customTestResult.tools.map((t) => t.name).join(", ")}
                    </div>
                  )}

                  <div className="flex items-center justify-end gap-2">
                    <Button
                      size="xs"
                      variant="outline"
                      onClick={handleTestCustom}
                      disabled={testServer.isPending || !customUrl.trim()}
                      className="text-xs gap-1"
                    >
                      {testServer.isPending ? <Loader2 className="size-3 animate-spin" /> : <Wrench className="size-3" />}
                      <span>Test URL</span>
                    </Button>
                    <Button
                      size="xs"
                      onClick={handleCreateCustom}
                      disabled={createServer.isPending || !customName.trim() || !customUrl.trim()}
                      className="text-xs"
                    >
                      <span>Save Custom Server</span>
                    </Button>
                  </div>
                </div>
              )}
            </div>
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
