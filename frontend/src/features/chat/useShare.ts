import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";

export interface ShareResult {
  token?: string;
  shared?: boolean;
}

export function useShareState(id: string | null) {
  return useQuery({
    queryKey: ["share", id],
    queryFn: () => apiFetch<{ shared: boolean }>(`/api/conversations/${id}/shares`),
    enabled: id !== null,
  });
}

export function useShareConversation() {
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<ShareResult>(`/api/conversations/${id}/shares`, { method: "POST" }),
  });
}

export function useUnshareConversation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/api/conversations/${id}/shares`, { method: "DELETE" }),
    onSuccess: (_, id) => {
      void queryClient.invalidateQueries({ queryKey: ["share", id] });
    },
  });
}
