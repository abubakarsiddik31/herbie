import { useState } from "react";
import {
  CheckCircle2,
  ExternalLink,
  GitBranch,
  History,
  KeyRound,
  Lock,
  MessageSquare,
  Plus,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { apiFetch } from "@/lib/api";
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
  useCreateWorkflowCredential,
  useDeleteWorkflowCredential,
  useDisconnectToolOAuth,
  useToolAuditLogs,
  useToolOAuthProviders,
  useWorkflowCredentials,
} from "./useWorkflows";

interface CredentialsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CredentialsDialog({ open, onOpenChange }: CredentialsDialogProps) {
  const [activeTab, setActiveTab] = useState<"vault" | "audit">("vault");
  const { data: credentials, isLoading } = useWorkflowCredentials();
  const { data: oauthProviders, isLoading: isOAuthLoading } = useToolOAuthProviders();
  const disconnectOAuth = useDisconnectToolOAuth();
  const { data: auditLogs, isLoading: isAuditLoading } = useToolAuditLogs(50);
  const createCred = useCreateWorkflowCredential();
  const deleteCred = useDeleteWorkflowCredential();

  const [isAdding, setIsAdding] = useState(false);
  const [name, setName] = useState("");
  const [type, setType] = useState<string>("bearer_token");
  const [secretVal, setSecretVal] = useState("");
  const [connectingProvider, setConnectingProvider] = useState<string | null>(null);

  async function handleConnectOAuth(providerId: string) {
    setConnectingProvider(providerId);
    try {
      const data = await apiFetch<{ url: string }>(`/api/tool-oauth/${providerId}/start`);
      if (data.url) {
        window.location.href = data.url;
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to start ${providerId} OAuth`);
      setConnectingProvider(null);
    }
  }

  function handleDisconnectOAuth(providerId: string) {
    disconnectOAuth.mutate(providerId, {
      onSuccess: () => toast.success(`Disconnected ${providerId}`),
      onError: (err) => toast.error(err instanceof Error ? err.message : `Failed to disconnect ${providerId}`),
    });
  }

  function handleAdd() {
    if (!name.trim() || !secretVal.trim()) {
      toast.error("Name and secret value are required");
      return;
    }
    const cleanName = name.trim().toLowerCase().replace(/\s+/g, "_");
    const dataMap: Record<string, string> = {};
    if (type === "bearer_token") {
      dataMap.token = secretVal.trim();
    } else if (type === "api_key") {
      dataMap.key = secretVal.trim();
    } else {
      dataMap.value = secretVal.trim();
    }

    createCred.mutate(
      { name: cleanName, type, data: dataMap },
      {
        onSuccess: () => {
          toast.success(`Credential "${cleanName}" saved`);
          setName("");
          setSecretVal("");
          setIsAdding(false);
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : "Failed to save credential");
        },
      }
    );
  }

  function handleDelete(id: string, credName: string) {
    deleteCred.mutate(id, {
      onSuccess: () => toast.success(`Deleted credential "${credName}"`),
      onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to delete credential"),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <div className="flex items-center justify-between pr-4">
            <DialogTitle className="flex items-center gap-2 text-base">
              <KeyRound className="size-4 text-primary" />
              <span>Credentials Vault & Tool Safety</span>
            </DialogTitle>
          </div>
          <DialogDescription className="text-xs">
            Manage encrypted credentials for AI integrations, connect OAuth providers, and inspect security audit logs.
          </DialogDescription>
        </DialogHeader>

        {/* View Switcher Tabs */}
        <div className="flex border-b border-border text-xs">
          <button
            type="button"
            onClick={() => setActiveTab("vault")}
            className={`flex items-center gap-1.5 px-3 py-2 font-medium border-b-2 transition-colors ${
              activeTab === "vault"
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            <Lock className="size-3.5" />
            <span>Credentials Vault</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveTab("audit")}
            className={`flex items-center gap-1.5 px-3 py-2 font-medium border-b-2 transition-colors ${
              activeTab === "audit"
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            <History className="size-3.5" />
            <span>Security Audit Trail</span>
          </button>
        </div>

        {activeTab === "vault" ? (
          <div className="space-y-4 pt-1">
            {/* Encryption & Safety Banner */}
            <div className="flex items-start gap-2.5 rounded-lg border border-emerald-600/30 bg-emerald-500/10 p-2.5 text-xs text-emerald-800 dark:text-emerald-300">
              <ShieldCheck className="size-4 shrink-0 mt-0.5" />
              <div>
                <span className="font-semibold">AES-256-GCM Encrypted at Rest</span>
                <p className="text-[11px] text-muted-foreground mt-0.5">
                  Stored secrets are encrypted using authenticated AES-GCM and decrypted in-memory strictly for node execution. Outgoing calls are guarded against SSRF and secrets are redacted from model history.
                </p>
              </div>
            </div>

            {/* 1-Click OAuth Tool Integrations */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-foreground">One-Click OAuth Integrations</span>
                <span className="text-[10px] text-muted-foreground">Zero manual PATs</span>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                {/* GitHub Integration Card */}
                {(() => {
                  const gh = oauthProviders?.find((p) => p.id === "github");
                  const isConnected = gh?.connected;
                  return (
                    <div className="flex flex-col justify-between rounded-lg border border-border bg-card p-3 text-xs shadow-2xs">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <div className="p-1 rounded bg-muted">
                            <GitBranch className="size-4" />
                          </div>
                          <div>
                            <p className="font-semibold text-foreground">GitHub</p>
                            <p className="text-[10px] text-muted-foreground">repo, read:user</p>
                          </div>
                        </div>
                        {isConnected ? (
                          <Badge variant="outline" className="border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400 gap-1 text-[10px]">
                            <CheckCircle2 className="size-3" />
                            <span>Connected</span>
                          </Badge>
                        ) : (
                          <Badge variant="outline" className="text-muted-foreground text-[10px]">
                            {gh?.configured ? "Ready" : "Not Configured"}
                          </Badge>
                        )}
                      </div>

                      <div className="mt-3 pt-2 border-t border-border/60 flex justify-end">
                        {isConnected ? (
                          <Button
                            variant="ghost"
                            size="xs"
                            onClick={() => handleDisconnectOAuth("github")}
                            className="text-muted-foreground hover:text-destructive text-[11px]"
                          >
                            Disconnect
                          </Button>
                        ) : (
                          <Button
                            variant="outline"
                            size="xs"
                            disabled={!gh?.configured || connectingProvider === "github" || isOAuthLoading}
                            onClick={() => handleConnectOAuth("github")}
                            className="gap-1 text-[11px]"
                          >
                            <ExternalLink className="size-3" />
                            <span>{connectingProvider === "github" ? "Connecting..." : "Connect Account"}</span>
                          </Button>
                        )}
                      </div>
                    </div>
                  );
                })()}

                {/* Slack Integration Card */}
                {(() => {
                  const slk = oauthProviders?.find((p) => p.id === "slack");
                  const isConnected = slk?.connected;
                  return (
                    <div className="flex flex-col justify-between rounded-lg border border-border bg-card p-3 text-xs shadow-2xs">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <div className="p-1 rounded bg-muted">
                            <MessageSquare className="size-4 text-emerald-600 dark:text-emerald-400" />
                          </div>
                          <div>
                            <p className="font-semibold text-foreground">Slack</p>
                            <p className="text-[10px] text-muted-foreground">webhooks, chat:write</p>
                          </div>
                        </div>
                        {isConnected ? (
                          <Badge variant="outline" className="border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400 gap-1 text-[10px]">
                            <CheckCircle2 className="size-3" />
                            <span>Connected</span>
                          </Badge>
                        ) : (
                          <Badge variant="outline" className="text-muted-foreground text-[10px]">
                            {slk?.configured ? "Ready" : "Not Configured"}
                          </Badge>
                        )}
                      </div>

                      <div className="mt-3 pt-2 border-t border-border/60 flex justify-end">
                        {isConnected ? (
                          <Button
                            variant="ghost"
                            size="xs"
                            onClick={() => handleDisconnectOAuth("slack")}
                            className="text-muted-foreground hover:text-destructive text-[11px]"
                          >
                            Disconnect
                          </Button>
                        ) : (
                          <Button
                            variant="outline"
                            size="xs"
                            disabled={!slk?.configured || connectingProvider === "slack" || isOAuthLoading}
                            onClick={() => handleConnectOAuth("slack")}
                            className="gap-1 text-[11px]"
                          >
                            <ExternalLink className="size-3" />
                            <span>{connectingProvider === "slack" ? "Connecting..." : "Connect Account"}</span>
                          </Button>
                        )}
                      </div>
                    </div>
                  );
                })()}
              </div>
            </div>

            {/* Custom Credentials List */}
            <div className="space-y-2">
              <span className="text-xs font-semibold text-foreground">Stored Credentials & Secrets</span>
              <ScrollArea className="max-h-48 pr-2">
                {isLoading && <p className="text-xs text-muted-foreground">Loading credentials...</p>}
                {!isLoading && (!credentials || credentials.length === 0) && !isAdding && (
                  <p className="py-3 text-center text-xs text-muted-foreground border rounded-lg border-dashed">
                    No custom credentials stored yet. Reference secrets in nodes via <code className="font-mono bg-muted px-1 rounded">{"{{ $credentials.name.token }}"}</code>.
                  </p>
                )}
                <div className="space-y-2">
                  {credentials?.map((c) => (
                    <div
                      key={c.id}
                      className="flex items-center justify-between rounded-lg border border-border bg-card p-2.5 text-xs shadow-2xs"
                    >
                      <div>
                        <div className="flex items-center gap-1.5">
                          <p className="font-semibold text-foreground font-mono">{c.name}</p>
                          {c.provider && (
                            <Badge variant="secondary" className="text-[9px] py-0 px-1">
                              {c.provider}
                            </Badge>
                          )}
                        </div>
                        <p className="text-[10px] text-muted-foreground uppercase">{c.type}</p>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => handleDelete(c.id, c.name)}
                        className="text-muted-foreground hover:text-destructive"
                      >
                        <Trash2 className="size-3.5" />
                      </Button>
                    </div>
                  ))}
                </div>
              </ScrollArea>
            </div>

            {/* Add Custom Credential Toggle */}
            {!isAdding ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setIsAdding(true)}
                className="w-full text-xs gap-1.5"
              >
                <Plus className="size-3.5" />
                <span>Add Custom API Secret</span>
              </Button>
            ) : (
              <div className="rounded-xl border border-border/80 bg-muted/30 p-3.5 space-y-3 text-xs">
                <div className="space-y-1">
                  <Label htmlFor="cred-name" className="text-xs">Credential Name</Label>
                  <Input
                    id="cred-name"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="e.g. github_token, slack_webhook"
                    className="h-8 font-mono text-xs"
                  />
                </div>

                <div className="space-y-1">
                  <Label className="text-xs">Credential Type</Label>
                  <select
                    value={type}
                    onChange={(e) => setType(e.target.value)}
                    className="h-8 w-full rounded-md border border-input bg-background px-2 text-xs outline-none focus-visible:border-ring"
                  >
                    <option value="bearer_token">Bearer Token (OAuth / Personal Access)</option>
                    <option value="api_key">API Key</option>
                    <option value="custom_secret">Custom Secret</option>
                  </select>
                </div>

                <div className="space-y-1">
                  <Label htmlFor="cred-val" className="text-xs">Secret Value</Label>
                  <Input
                    id="cred-val"
                    type="password"
                    value={secretVal}
                    onChange={(e) => setSecretVal(e.target.value)}
                    placeholder="Paste secret token here..."
                    className="h-8 font-mono text-xs"
                  />
                </div>

                <div className="flex justify-end gap-2 pt-1">
                  <Button variant="ghost" size="sm" onClick={() => setIsAdding(false)}>
                    Cancel
                  </Button>
                  <Button variant="brand" size="sm" onClick={handleAdd} disabled={createCred.isPending}>
                    {createCred.isPending ? "Saving..." : "Encrypt & Save"}
                  </Button>
                </div>
              </div>
            )}
          </div>
        ) : (
          /* Security Audit Trail Tab */
          <div className="space-y-3 pt-1">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-foreground">Recent Security Invocations</span>
              <span className="text-[10px] text-muted-foreground">Last 50 actions</span>
            </div>

            <ScrollArea className="max-h-80 pr-2">
              {isAuditLoading && <p className="text-xs text-muted-foreground py-4 text-center">Loading audit records...</p>}
              {!isAuditLoading && (!auditLogs || auditLogs.length === 0) && (
                <p className="py-6 text-center text-xs text-muted-foreground border rounded-lg border-dashed">
                  No audit logs recorded yet. Tool and workflow executions will automatically appear here.
                </p>
              )}
              <div className="space-y-2">
                {auditLogs?.map((log) => (
                  <div
                    key={log.id}
                    className="rounded-lg border border-border bg-card p-2.5 text-xs shadow-2xs space-y-1"
                  >
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-1.5">
                        <Badge
                          variant="outline"
                          className={
                            log.callerType === "chat_agent"
                              ? "border-primary/30 bg-primary/10 text-primary text-[10px]"
                              : "border-border text-muted-foreground text-[10px]"
                          }
                        >
                          {log.callerType === "chat_agent" ? "AI Agent" : log.callerType}
                        </Badge>
                        <span className="font-semibold font-mono text-foreground">{log.toolName}</span>
                      </div>
                      <Badge
                        variant="outline"
                        className={
                          log.status === "success" || log.status === "approved"
                            ? "border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400 text-[10px]"
                            : log.status === "approval_pending"
                            ? "border-amber-600/30 bg-amber-600/10 text-amber-700 dark:text-amber-400 text-[10px]"
                            : "border-rose-600/30 bg-rose-600/10 text-rose-700 dark:text-rose-400 text-[10px]"
                        }
                      >
                        {log.status}
                      </Badge>
                    </div>

                    <div className="text-[11px] text-muted-foreground font-mono truncate">
                      {log.action}: {log.inputSummary || "no arguments"}
                    </div>

                    <div className="flex items-center justify-between text-[10px] text-muted-foreground pt-0.5">
                      <span>{log.durationMs > 0 ? `${log.durationMs}ms` : "instant"}</span>
                      <span>{new Date(log.createdAt).toLocaleString()}</span>
                    </div>
                  </div>
                ))}
              </div>
            </ScrollArea>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
