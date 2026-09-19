import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import type { ToolAuditLog, ToolOAuthProvider, Workflow, WorkflowCredential, WorkflowRun } from "@/lib/types";

export function useWorkflows() {
  return useQuery({
    queryKey: ["workflows"],
    queryFn: async () => {
      const data = await apiFetch<{ workflows: Workflow[] }>("/api/workflows");
      return data.workflows ?? [];
    },
  });
}

export function useWorkflow(id: string | undefined) {
  return useQuery({
    queryKey: ["workflows", id],
    queryFn: async () => {
      if (!id) return null;
      return apiFetch<Workflow>(`/api/workflows/${id}`);
    },
    enabled: Boolean(id),
  });
}

export function useCreateWorkflow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (workflow: Partial<Workflow>) =>
      apiFetch<Workflow>("/api/workflows", {
        method: "POST",
        json: workflow,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workflows"] });
    },
  });
}

export function useUpdateWorkflow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: Partial<Workflow> }) =>
      apiFetch<Workflow>(`/api/workflows/${id}`, {
        method: "PATCH",
        json: patch,
      }),
    onSuccess: (_, vars) => {
      void queryClient.invalidateQueries({ queryKey: ["workflows"] });
      void queryClient.invalidateQueries({ queryKey: ["workflows", vars.id] });
    },
  });
}

export function useDeleteWorkflow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/api/workflows/${id}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workflows"] });
    },
  });
}

export function useWorkflowRuns(workflowId: string | undefined) {
  return useQuery({
    queryKey: ["workflows", workflowId, "runs"],
    queryFn: async () => {
      if (!workflowId) return [];
      const data = await apiFetch<{ runs: WorkflowRun[] }>(`/api/workflows/${workflowId}/runs`);
      return data.runs ?? [];
    },
    enabled: Boolean(workflowId),
  });
}

export function useWorkflowCredentials() {
  return useQuery({
    queryKey: ["workflow-credentials"],
    queryFn: async () => {
      const data = await apiFetch<{ credentials: WorkflowCredential[] }>("/api/workflow-credentials");
      return data.credentials ?? [];
    },
  });
}

export function useCreateWorkflowCredential() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (cred: { name: string; type: string; data: Record<string, string> }) =>
      apiFetch<WorkflowCredential>("/api/workflow-credentials", {
        method: "POST",
        json: cred,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workflow-credentials"] });
    },
  });
}

export function useDeleteWorkflowCredential() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/api/workflow-credentials/${id}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workflow-credentials"] });
      void queryClient.invalidateQueries({ queryKey: ["tool-oauth-providers"] });
    },
  });
}

export function useToolOAuthProviders() {
  return useQuery({
    queryKey: ["tool-oauth-providers"],
    queryFn: async () => {
      const data = await apiFetch<{ providers: ToolOAuthProvider[] }>("/api/tool-oauth/providers");
      return data.providers ?? [];
    },
  });
}

export function useConfigureToolOAuth() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ provider, ...body }: { provider: string; clientId?: string; clientSecret?: string; token?: string; refreshToken?: string }) =>
      apiFetch<{ ok: boolean }>(`/api/tool-oauth/${provider}/config`, {
        method: "POST",
        json: body,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["tool-oauth-providers"] });
      void queryClient.invalidateQueries({ queryKey: ["workflow-credentials"] });
    },
  });
}

export function useDisconnectToolOAuth() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (provider: string) =>
      apiFetch<{ ok: boolean }>(`/api/tool-oauth/${provider}/disconnect`, {
        method: "POST",
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["tool-oauth-providers"] });
      void queryClient.invalidateQueries({ queryKey: ["workflow-credentials"] });
      void queryClient.invalidateQueries({ queryKey: ["tool-audit-logs"] });
    },
  });
}

export function useToolAuditLogs(limit = 50) {
  return useQuery({
    queryKey: ["tool-audit-logs", limit],
    queryFn: async () => {
      const data = await apiFetch<{ audits: ToolAuditLog[] }>(`/api/tool-audit-logs?limit=${limit}`);
      return data.audits ?? [];
    },
  });
}
