import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "@/lib/api";

export interface CatalogTool {
  name: string;
  description?: string;
  inputSchema: Record<string, unknown>;
}

export interface CatalogApp {
  id: string;
  name: string;
  mention: string;
  category: string;
  description: string;
  icon: string;
  tools: CatalogTool[];
  requiresAuth: boolean;
  authType?: string;
  authPrompt?: string;
  connected: boolean;
  connectedAt?: string;
  serverId?: string;
}

export function useMCPCatalog() {
  return useQuery({
    queryKey: ["mcp-catalog"],
    queryFn: async () => {
      const res = await apiFetch<{ catalog: CatalogApp[] }>("/api/mcp/catalog");
      return res.catalog;
    },
  });
}

export function useLinkCatalogApp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ appId, token }: { appId: string; token?: string }) =>
      apiFetch<{ ok: boolean }>(`/api/mcp/catalog/${appId}/link`, {
        method: "POST",
        json: { token },
      }),
    onSuccess: (_, { appId }) => {
      void qc.invalidateQueries({ queryKey: ["mcp-catalog"] });
      void qc.invalidateQueries({ queryKey: ["mcp-servers"] });
      void qc.invalidateQueries({ queryKey: ["tool-oauth-providers"] });
      toast.success(`Linked ${appId} MCP in 1 click!`);
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to link app");
    },
  });
}

export function useUnlinkCatalogApp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (appId: string) =>
      apiFetch<{ ok: boolean }>(`/api/mcp/catalog/${appId}/unlink`, {
        method: "POST",
      }),
    onSuccess: (_, appId) => {
      void qc.invalidateQueries({ queryKey: ["mcp-catalog"] });
      void qc.invalidateQueries({ queryKey: ["mcp-servers"] });
      void qc.invalidateQueries({ queryKey: ["tool-oauth-providers"] });
      toast.success(`Unlinked ${appId} MCP`);
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to unlink app");
    },
  });
}
