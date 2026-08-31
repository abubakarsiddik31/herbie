import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ApiError, apiFetch } from "@/lib/api";
import type { UserTool } from "@/lib/types";

export function useTools() {
  return useQuery({
    queryKey: ["tools"],
    queryFn: () => apiFetch<UserTool[]>("/api/tools"),
  });
}

export function useCreateTool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (tool: Partial<UserTool>) =>
      apiFetch<UserTool>("/api/tools", { method: "POST", json: tool }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["tools"] }),
    onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to create tool"),
  });
}

export function useUpdateTool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: Record<string, unknown> }) =>
      apiFetch<UserTool>(`/api/tools/${id}`, { method: "PATCH", json: patch }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["tools"] }),
    onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to update tool"),
  });
}

export function useDeleteTool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/api/tools/${id}`, { method: "DELETE" }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["tools"] }),
    onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to delete tool"),
  });
}
