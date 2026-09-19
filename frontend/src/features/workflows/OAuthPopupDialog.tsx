import { useState } from "react";
import {
  Calendar,
  Check,
  Copy,
  ExternalLink,
  GitBranch,
  Key,
  Loader2,
  MessageSquare,
  ShieldAlert,
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
import { useConfigureToolOAuth, useToolOAuthProviders } from "./useWorkflows";
import { apiFetch } from "@/lib/api";
import { cn } from "@/lib/utils";

export interface OAuthPopupDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: "google_calendar" | "github" | "slack" | string;
}

const PROVIDER_METAS: Record<
  string,
  { name: string; icon: "calendar" | "github" | "slack"; authUrlHint: string }
> = {
  google_calendar: {
    name: "Google Calendar",
    icon: "calendar",
    authUrlHint: "Google Cloud Console → APIs & Services → Credentials",
  },
  github: {
    name: "GitHub",
    icon: "github",
    authUrlHint: "GitHub Settings → Developer Settings → OAuth Apps",
  },
  slack: {
    name: "Slack",
    icon: "slack",
    authUrlHint: "api.slack.com/apps → OAuth & Permissions",
  },
};

export function OAuthPopupDialog({ open, onOpenChange, providerId }: OAuthPopupDialogProps) {
  const [tab, setTab] = useState<"oauth" | "token">("oauth");
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [token, setToken] = useState("");
  const [refreshToken, setRefreshToken] = useState("");
  const [copied, setCopied] = useState(false);
  const [authorizing, setAuthorizing] = useState(false);

  const { data: providers } = useToolOAuthProviders();
  const configureOAuth = useConfigureToolOAuth();

  const meta = PROVIDER_METAS[providerId] || {
    name: providerId,
    icon: "calendar",
    authUrlHint: "OAuth Developer Portal",
  };

  const provider = providers?.find((p) => p.id === providerId);
  const redirectUri = `${window.location.origin}/api/tool-oauth/${providerId}/callback`;

  function handleCopyRedirectUri() {
    void navigator.clipboard.writeText(redirectUri);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
    toast.success("Callback URL copied to clipboard");
  }

  function launchOAuthPopup(authUrl: string) {
    const width = 600;
    const height = 700;
    const left = window.screenX + (window.outerWidth - width) / 2;
    const top = window.screenY + (window.outerHeight - height) / 2;

    const popup = window.open(
      authUrl,
      "oauth_popup",
      `width=${width},height=${height},left=${left},top=${top},menubar=no,toolbar=no,status=no`,
    );

    const timer = setInterval(() => {
      if (!popup || popup.closed) {
        clearInterval(timer);
        setAuthorizing(false);
        onOpenChange(false);
      }
    }, 1000);

    const handleMessage = (e: MessageEvent) => {
      if (e.data && (e.data.type === "tool_oauth_success" || e.data.type === "tool_oauth_error")) {
        clearInterval(timer);
        window.removeEventListener("message", handleMessage);
        if (popup && !popup.closed) {
          popup.close();
        }
        setAuthorizing(false);
        onOpenChange(false);
        if (e.data.type === "tool_oauth_success") {
          toast.success(`${meta.name} connected successfully!`);
        } else {
          toast.error(`Authentication failed: ${e.data.error || "unknown"}`);
        }
      }
    };
    window.addEventListener("message", handleMessage);
  }

  async function handleStartOAuth() {
    setAuthorizing(true);
    try {
      // If user filled in Client ID/Secret, save them first
      if (clientId.trim() && clientSecret.trim()) {
        await configureOAuth.mutateAsync({
          provider: providerId,
          clientId: clientId.trim(),
          clientSecret: clientSecret.trim(),
        });
      }

      const dest = window.location.pathname + window.location.search;
      const data = await apiFetch<{ url: string }>(
        `/api/tool-oauth/${providerId}/start?return_to=${encodeURIComponent(dest)}`,
      );
      if (data.url) {
        launchOAuthPopup(data.url);
      } else {
        setAuthorizing(false);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to start ${meta.name} authorization`);
      setAuthorizing(false);
    }
  }

  async function handleSaveToken() {
    if (!token.trim()) {
      toast.error("Please enter an access token");
      return;
    }
    try {
      await configureOAuth.mutateAsync({
        provider: providerId,
        token: token.trim(),
        refreshToken: refreshToken.trim() || undefined,
      });
      toast.success(`${meta.name} credentials saved!`);
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save token");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md p-6 gap-0">
        <DialogHeader className="pb-3 border-b border-border/60">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2.5">
              <div className="flex size-8 items-center justify-center rounded-xl bg-muted/80 border border-border/80 text-foreground">
                {meta.icon === "calendar" && <Calendar className="size-4 text-sky-500" />}
                {meta.icon === "github" && <GitBranch className="size-4 text-foreground" />}
                {meta.icon === "slack" && <MessageSquare className="size-4 text-emerald-500" />}
              </div>
              <div>
                <DialogTitle className="text-sm font-semibold">
                  Connect {meta.name}
                </DialogTitle>
                <DialogDescription className="text-xs text-muted-foreground mt-0.5">
                  Authorize with {meta.name} via popup or access token.
                </DialogDescription>
              </div>
            </div>

            <Badge
              variant="outline"
              className={cn(
                "text-[10px]",
                provider?.configured
                  ? "border-emerald-500/30 text-emerald-600 dark:text-emerald-400"
                  : "border-amber-500/30 text-amber-600 dark:text-amber-400"
              )}
            >
              {provider?.configured ? "Ready" : "Credentials Needed"}
            </Badge>
          </div>

          {/* Mode Switcher */}
          <div className="flex items-center gap-1.5 pt-3">
            <button
              type="button"
              onClick={() => setTab("oauth")}
              className={cn(
                "px-2.5 py-1 text-xs font-medium rounded-lg transition-colors",
                tab === "oauth"
                  ? "bg-primary/10 text-primary font-semibold"
                  : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
              )}
            >
              OAuth 2.0 Popup
            </button>
            <button
              type="button"
              onClick={() => setTab("token")}
              className={cn(
                "px-2.5 py-1 text-xs font-medium rounded-lg transition-colors",
                tab === "token"
                  ? "bg-primary/10 text-primary font-semibold"
                  : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
              )}
            >
              Direct Token
            </button>
          </div>
        </DialogHeader>

        <div className="py-4 space-y-4">
          {tab === "oauth" && (
            <div className="space-y-3.5 text-xs">
              {!provider?.configured && (
                <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 space-y-1.5">
                  <div className="flex items-center gap-1.5 font-medium text-amber-900 dark:text-amber-200">
                    <ShieldAlert className="size-3.5 text-amber-600 shrink-0" />
                    <span>OAuth Credentials Required</span>
                  </div>
                  <p className="text-[11px] text-muted-foreground leading-relaxed">
                    Enter your Client ID & Secret from {meta.authUrlHint}. We will save them securely to launch your sign-in popup.
                  </p>
                </div>
              )}

              {/* Callback URI helper */}
              <div className="space-y-1">
                <Label className="text-[11px] text-muted-foreground">Authorized Redirect URI</Label>
                <div className="flex items-center gap-1.5 rounded-lg border border-border/80 bg-muted/40 px-2.5 py-1.5 font-mono text-[10px]">
                  <span className="truncate flex-1 text-foreground">{redirectUri}</span>
                  <button
                    type="button"
                    onClick={handleCopyRedirectUri}
                    className="shrink-0 p-1 text-muted-foreground hover:text-foreground"
                    title="Copy URI"
                  >
                    {copied ? <Check className="size-3 text-emerald-500" /> : <Copy className="size-3" />}
                  </button>
                </div>
              </div>

              {!provider?.configured && (
                <div className="space-y-2 pt-1">
                  <div className="space-y-1">
                    <Label htmlFor="oauth-client-id" className="text-xs">
                      Client ID
                    </Label>
                    <Input
                      id="oauth-client-id"
                      placeholder="e.g. 123456789.apps.googleusercontent.com"
                      value={clientId}
                      onChange={(e) => setClientId(e.target.value)}
                      className="h-8 text-xs font-mono"
                    />
                  </div>

                  <div className="space-y-1">
                    <Label htmlFor="oauth-client-secret" className="text-xs">
                      Client Secret
                    </Label>
                    <Input
                      id="oauth-client-secret"
                      type="password"
                      placeholder="e.g. GOCSPX-..."
                      value={clientSecret}
                      onChange={(e) => setClientSecret(e.target.value)}
                      className="h-8 text-xs font-mono"
                    />
                  </div>
                </div>
              )}

              <div className="pt-2 border-t border-border/60 flex justify-end">
                <Button
                  size="sm"
                  onClick={handleStartOAuth}
                  disabled={authorizing || (!provider?.configured && (!clientId.trim() || !clientSecret.trim()))}
                  className="text-xs gap-1.5 w-full sm:w-auto"
                >
                  {authorizing ? (
                    <>
                      <Loader2 className="size-3.5 animate-spin" />
                      <span>Opening Authorization Popup…</span>
                    </>
                  ) : (
                    <>
                      <ExternalLink className="size-3.5" />
                      <span>Open {meta.name} Popup</span>
                    </>
                  )}
                </Button>
              </div>
            </div>
          )}

          {tab === "token" && (
            <div className="space-y-3 text-xs">
              <div className="rounded-xl border border-border/80 bg-muted/30 p-3 text-[11px] text-muted-foreground leading-relaxed">
                Paste an existing OAuth Access Token, Refresh Token, or Personal Access Token directly to connect without setting up OAuth credentials.
              </div>

              <div className="space-y-2">
                <div className="space-y-1">
                  <Label htmlFor="direct-token" className="text-xs">
                    Access Token / API Key
                  </Label>
                  <Input
                    id="direct-token"
                    type="password"
                    placeholder="e.g. ya29... or ghp_..."
                    value={token}
                    onChange={(e) => setToken(e.target.value)}
                    className="h-8 text-xs font-mono"
                  />
                </div>

                <div className="space-y-1">
                  <Label htmlFor="direct-refresh" className="text-xs flex items-center justify-between">
                    <span>Refresh Token (Optional)</span>
                    <span className="text-[10px] text-muted-foreground">For automatic renewal</span>
                  </Label>
                  <Input
                    id="direct-refresh"
                    type="password"
                    placeholder="e.g. 1//04..."
                    value={refreshToken}
                    onChange={(e) => setRefreshToken(e.target.value)}
                    className="h-8 text-xs font-mono"
                  />
                </div>
              </div>

              <div className="pt-2 border-t border-border/60 flex justify-end">
                <Button
                  size="sm"
                  onClick={handleSaveToken}
                  disabled={configureOAuth.isPending || !token.trim()}
                  className="text-xs gap-1.5 w-full sm:w-auto"
                >
                  {configureOAuth.isPending ? (
                    <>
                      <Loader2 className="size-3.5 animate-spin" />
                      <span>Saving…</span>
                    </>
                  ) : (
                    <>
                      <Key className="size-3.5" />
                      <span>Save & Connect</span>
                    </>
                  )}
                </Button>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
