import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "@/lib/api";

export interface MCPServer {
  id: string;
  userId: string;
  name: string;
  url: string;
  transport: string;
  appId?: string | null;
  enabled: boolean;
  headers?: Record<string, string>;
  createdAt: string;
  updatedAt: string;
}

export interface MCPTestResult {
  ok: boolean;
  count: number;
  tools: Array<{
    name: string;
    description?: string;
    inputSchema: Record<string, unknown>;
  }>;
}

export function useMCPServers() {
  return useQuery({
    queryKey: ["mcp-servers"],
    queryFn: async () => {
      const res = await apiFetch<{ servers: MCPServer[] }>("/api/mcp/servers");
      return res?.servers ?? [];
    },
  });
}

export function useCreateMCPServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; url: string; appId?: string | null; headers?: Record<string, string> }) =>
      apiFetch<{ server: MCPServer }>("/api/mcp/servers", {
        method: "POST",
        json: body,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["mcp-servers"] });
      void qc.invalidateQueries({ queryKey: ["tool-oauth-providers"] });
      toast.success("MCP server registered successfully");
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to add MCP server");
    },
  });
}

export function useUpdateMCPServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; name?: string; url?: string; appId?: string | null; headers?: Record<string, string> }) =>
      apiFetch<{ server: MCPServer }>(`/api/mcp/servers/${id}`, {
        method: "PUT",
        json: body,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["mcp-servers"] });
      void qc.invalidateQueries({ queryKey: ["tool-oauth-providers"] });
      toast.success("MCP server updated");
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to update MCP server");
    },
  });
}

export function useUnlinkAppMCPServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<{ server: MCPServer }>(`/api/mcp/servers/${id}/unlink-app`, {
        method: "POST",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["mcp-servers"] });
      void qc.invalidateQueries({ queryKey: ["tool-oauth-providers"] });
      toast.success("App unlinked from MCP server");
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to unlink app from MCP server");
    },
  });
}

export function useToggleMCPServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<{ server: MCPServer }>(`/api/mcp/servers/${id}/toggle`, {
        method: "POST",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["mcp-servers"] });
      void qc.invalidateQueries({ queryKey: ["tool-oauth-providers"] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to toggle MCP server");
    },
  });
}

export function useDeleteMCPServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<{ ok: boolean }>(`/api/mcp/servers/${id}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["mcp-servers"] });
      void qc.invalidateQueries({ queryKey: ["tool-oauth-providers"] });
      toast.success("MCP server deleted");
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to delete MCP server");
    },
  });
}

export function useTestMCPServer() {
  return useMutation({
    mutationFn: (body: { url: string; headers?: Record<string, string> }) =>
      apiFetch<MCPTestResult>("/api/mcp/servers/test", {
        method: "POST",
        json: body,
      }),
  });
}
