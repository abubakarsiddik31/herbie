import { useState } from "react";
import { ExternalLink, CheckCircle2, Loader2, Plug, SlidersHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { apiFetch } from "@/lib/api";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { useToolOAuthProviders } from "@/features/workflows/useWorkflows";
import { useLinkCatalogApp } from "@/features/mcp/useMCPCatalog";
import { OAuthPopupDialog } from "@/features/workflows/OAuthPopupDialog";
import { MentionAppIcon } from "./MentionMenu";

export interface AppConnectCardProps {
  providerId: "google_calendar" | "github" | "slack" | string;
  className?: string;
  returnTo?: string;
}

const PROVIDER_METADATA: Record<
  string,
  { name: string; description: string; icon: "calendar" | "github" | "slack" }
> = {
  google_calendar: {
    name: "Google Calendar",
    description: "Allow Herbie to access your calendar to view schedule and create events.",
    icon: "calendar",
  },
  github: {
    name: "GitHub",
    description: "Allow Herbie to search repositories, view pull requests, and file issues.",
    icon: "github",
  },
  slack: {
    name: "Slack",
    description: "Allow Herbie to send notifications and post updates to your Slack channels.",
    icon: "slack",
  },
};

export function AppConnectCard({ providerId, className, returnTo }: AppConnectCardProps) {
  const { data: providers } = useToolOAuthProviders();
  const linkCatalogApp = useLinkCatalogApp();
  const [connecting, setConnecting] = useState(false);
  const [linkDialogOpen, setLinkDialogOpen] = useState(false);

  const meta = PROVIDER_METADATA[providerId] || {
    name: providerId,
    description: `Connect ${providerId} to allow Herbie to run actions on your behalf.`,
    icon: "calendar" as const,
  };

  const provider = providers?.find((p) => p.id === providerId);
  const isConnected = !!provider?.connected;
  const isConfigured = provider ? provider.configured : true;

  async function handleOneClickLink() {
    await linkCatalogApp.mutateAsync({ appId: providerId });
  }

  async function handleConnect() {
    setConnecting(true);
    try {
      const dest = returnTo || window.location.pathname + window.location.search;
      const data = await apiFetch<{ url: string }>(
        `/api/tool-oauth/${providerId}/start?return_to=${encodeURIComponent(dest)}`,
      );
      if (data.url) {
        window.location.href = data.url;
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to start ${meta.name} connection`);
      setConnecting(false);
    }
  }

  if (isConnected) {
    return (
      <div
        className={cn(
          "my-2 flex items-center justify-between gap-3 rounded-xl border border-emerald-600/30 bg-emerald-500/10 p-3 text-xs shadow-xs",
          className,
        )}
      >
        <div className="flex items-center gap-2.5">
          <div className="flex size-7 items-center justify-center rounded-lg bg-emerald-600/20 text-emerald-700 dark:text-emerald-300">
            <MentionAppIcon name={meta.icon} />
          </div>
          <div>
            <div className="flex items-center gap-1.5">
              <span className="font-semibold text-emerald-950 dark:text-emerald-100">{meta.name}</span>
              <Badge variant="outline" className="border-emerald-600/40 text-emerald-700 dark:text-emerald-300 text-[10px] gap-1 px-1.5 py-0 font-normal">
                <CheckCircle2 className="size-2.5" />
                <span>{provider?.connectedVia === "mcp" ? "Connected via MCP" : "Connected"}</span>
              </Badge>
            </div>
            <p className="text-[11px] text-muted-foreground mt-0.5">Ready for chat prompts and automated actions.</p>
          </div>
        </div>

        <Button
          size="xs"
          variant="outline"
          onClick={() => setLinkDialogOpen(true)}
          className="text-[11px] h-7 gap-1 text-muted-foreground hover:text-foreground"
        >
          <span>Manage</span>
        </Button>

        <OAuthPopupDialog
          open={linkDialogOpen}
          onOpenChange={setLinkDialogOpen}
          providerId={providerId}
        />
      </div>
    );
  }

  return (
    <>
      <div
        className={cn(
          "my-2 rounded-2xl border border-border/80 bg-card p-3.5 shadow-md space-y-3 max-w-md",
          className,
        )}
      >
        <div className="flex items-start gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-xl border border-border/80 bg-muted/50">
            <MentionAppIcon name={meta.icon} className="size-4" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center justify-between gap-2">
              <h4 className="font-semibold text-foreground text-xs">{meta.name} Connection Required</h4>
              <Badge variant="outline" className="border-purple-500/40 bg-purple-500/10 text-purple-700 dark:text-purple-400 text-[10px] font-normal">
                Integration
              </Badge>
            </div>
            <p className="text-[11px] text-muted-foreground mt-1 leading-relaxed">
              {meta.description} Connect your account to enable Herbie to take actions on your behalf.
            </p>
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 pt-1 border-t border-border/50">
          <Button
            size="xs"
            variant="ghost"
            type="button"
            onClick={() => setLinkDialogOpen(true)}
            className="gap-1 text-xs text-muted-foreground hover:text-foreground h-7 px-2"
          >
            <SlidersHorizontal className="size-3" />
            <span>Configure</span>
          </Button>

          {providerId === "google_calendar" ? (
            <Button
              size="xs"
              variant="default"
              onClick={() => setLinkDialogOpen(true)}
              className="gap-1.5 text-xs font-medium shadow-xs h-7"
            >
              <ExternalLink className="size-3" />
              <span>Connect Google Calendar</span>
            </Button>
          ) : (
            <>
              {isConfigured && (
                <Button
                  size="xs"
                  variant="outline"
                  disabled={connecting}
                  onClick={handleConnect}
                  className="gap-1.5 text-xs font-medium h-7"
                >
                  {connecting ? (
                    <>
                      <Loader2 className="size-3 animate-spin" />
                      <span>Connecting…</span>
                    </>
                  ) : (
                    <>
                      <ExternalLink className="size-3" />
                      <span>OAuth Connect</span>
                    </>
                  )}
                </Button>
              )}
              <Button
                size="xs"
                variant="default"
                disabled={linkCatalogApp.isPending}
                onClick={handleOneClickLink}
                className="gap-1.5 text-xs font-medium bg-emerald-600 hover:bg-emerald-700 text-white dark:bg-emerald-600 dark:hover:bg-emerald-500 shadow-xs h-7 rounded-lg"
              >
                {linkCatalogApp.isPending ? (
                  <Loader2 className="size-3 animate-spin" />
                ) : (
                  <Plug className="size-3" />
                )}
                <span>Connect {meta.name}</span>
              </Button>
            </>
          )}
        </div>
      </div>

      <OAuthPopupDialog
        open={linkDialogOpen}
        onOpenChange={setLinkDialogOpen}
        providerId={providerId}
      />
    </>
  );
}
