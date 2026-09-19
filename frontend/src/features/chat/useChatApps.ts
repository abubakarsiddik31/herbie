import { useMemo } from "react";
import { useToolOAuthProviders, useWorkflows } from "@/features/workflows/useWorkflows";
import { useTools } from "@/features/tools/useTools";

export interface ChatApp {
  id: string;
  name: string;
  mention: string;
  description: string;
  type: "oauth" | "builtin" | "workflow" | "custom_tool";
  iconName: "calendar" | "github" | "slack" | "globe" | "workflow" | "wrench";
  connected: boolean;
  configured: boolean;
  requiresConnection: boolean;
}

export function useChatApps() {
  const { data: oauthProviders, isLoading: isOAuthLoading } = useToolOAuthProviders();
  const { data: workflows, isLoading: isWorkflowsLoading } = useWorkflows();
  const { data: customTools, isLoading: isToolsLoading } = useTools();

  const apps = useMemo<ChatApp[]>(() => {
    const list: ChatApp[] = [];

    // 1. Google Calendar
    const gcal = oauthProviders?.find((p) => p.id === "google_calendar");
    list.push({
      id: "google_calendar",
      name: "Google Calendar",
      mention: "calendar",
      description: "Check schedule, view upcoming meetings, and create events",
      type: "oauth",
      iconName: "calendar",
      connected: !!gcal?.connected,
      configured: gcal ? gcal.configured : true,
      requiresConnection: true,
    });

    // 2. GitHub
    const gh = oauthProviders?.find((p) => p.id === "github");
    list.push({
      id: "github",
      name: "GitHub",
      mention: "github",
      description: "Search repositories, manage pull requests, and file issues",
      type: "oauth",
      iconName: "github",
      connected: !!gh?.connected,
      configured: gh ? gh.configured : true,
      requiresConnection: true,
    });

    // 3. Slack
    const slk = oauthProviders?.find((p) => p.id === "slack");
    list.push({
      id: "slack",
      name: "Slack",
      mention: "slack",
      description: "Send notifications and broadcast updates to Slack channels",
      type: "oauth",
      iconName: "slack",
      connected: !!slk?.connected,
      configured: slk ? slk.configured : true,
      requiresConnection: true,
    });

    // 4. Web Search (Built-in)
    list.push({
      id: "web_search",
      name: "Web Search",
      mention: "web",
      description: "Real-time search across the public web for latest sources",
      type: "builtin",
      iconName: "globe",
      connected: true,
      configured: true,
      requiresConnection: false,
    });

    // 5. Active Workflows exposed as chat tools
    if (workflows) {
      for (const wf of workflows) {
        if (wf.exposeAsTool) {
          const mentionName = wf.toolName?.replace(/^workflow_/, "") || wf.name.toLowerCase().replace(/\s+/g, "_");
          list.push({
            id: `workflow_${wf.id}`,
            name: wf.name,
            mention: mentionName,
            description: wf.toolDescription || wf.description || "Automated multi-step workflow",
            type: "workflow",
            iconName: "workflow",
            connected: true,
            configured: true,
            requiresConnection: false,
          });
        }
      }
    }

    // 6. Custom HTTP Tools
    if (customTools) {
      for (const t of customTools) {
        if (t.enabled) {
          list.push({
            id: `tool_${t.id}`,
            name: t.name,
            mention: t.name.toLowerCase().replace(/\s+/g, "_"),
            description: t.description || "Custom HTTP API tool",
            type: "custom_tool",
            iconName: "wrench",
            connected: true,
            configured: true,
            requiresConnection: false,
          });
        }
      }
    }

    return list;
  }, [oauthProviders, workflows, customTools]);

  const filterApps = (query: string): ChatApp[] => {
    const q = query.toLowerCase().trim();
    if (!q) return apps;
    return apps.filter(
      (a) =>
        a.mention.toLowerCase().includes(q) ||
        a.name.toLowerCase().includes(q) ||
        a.description.toLowerCase().includes(q),
    );
  };

  const getAppByMention = (mention: string): ChatApp | undefined => {
    const clean = mention.replace(/^@/, "").toLowerCase().trim();
    return apps.find((a) => a.mention.toLowerCase() === clean || a.id.toLowerCase() === clean);
  };

  return {
    apps,
    filterApps,
    getAppByMention,
    isLoading: isOAuthLoading || isWorkflowsLoading || isToolsLoading,
  };
}

/**
 * Extracts any `connect:<provider>` links from text to render interactive connect cards.
 */
export function extractAppConnectProviders(text: string): string[] {
  const matches = text.matchAll(/connect:([a-zA-Z0-9_-]+)/g);
  const set = new Set<string>();
  for (const m of matches) {
    if (m[1]) {
      set.add(m[1]);
    }
  }
  return Array.from(set);
}
