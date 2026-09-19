import { useState } from "react";
import { ExternalLink, CheckCircle2, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { apiFetch } from "@/lib/api";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { useToolOAuthProviders } from "@/features/workflows/useWorkflows";
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
  const [connecting, setConnecting] = useState(false);

  const meta = PROVIDER_METADATA[providerId] || {
    name: providerId,
    description: `Connect ${providerId} to allow Herbie to run actions on your behalf.`,
    icon: "calendar" as const,
  };

  const provider = providers?.find((p) => p.id === providerId);
  const isConnected = !!provider?.connected;

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
                <span>Connected</span>
              </Badge>
            </div>
            <p className="text-[11px] text-muted-foreground mt-0.5">Ready for chat prompts and automated actions.</p>
          </div>
        </div>
      </div>
    );
  }

  return (
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
            <Badge variant="outline" className="border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400 text-[10px] font-normal">
              1-Click Setup
            </Badge>
          </div>
          <p className="text-[11px] text-muted-foreground mt-1 leading-relaxed">
            {meta.description}
          </p>
        </div>
      </div>

      <div className="flex items-center justify-end gap-2 pt-1 border-t border-border/50">
        <Button
          size="xs"
          variant="default"
          disabled={connecting}
          onClick={handleConnect}
          className="gap-1.5 text-xs font-medium shadow-xs"
        >
          {connecting ? (
            <>
              <Loader2 className="size-3 animate-spin" />
              <span>Redirecting to {meta.name}…</span>
            </>
          ) : (
            <>
              <ExternalLink className="size-3" />
              <span>Connect {meta.name}</span>
            </>
          )}
        </Button>
      </div>
    </div>
  );
}
