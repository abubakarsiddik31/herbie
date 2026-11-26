import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import type { ConversationSummary } from "@/lib/types";

export function useConversationSummary(conversationId?: string) {
  return useQuery({
    queryKey: ["summary", conversationId],
    queryFn: () => apiFetch<ConversationSummary>(`/api/conversations/${conversationId}/summary`),
    enabled: Boolean(conversationId),
  });
}

export function useSaveSummary(conversationId?: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (summary: string) =>
      apiFetch<void>(`/api/conversations/${conversationId}/summary`, {
        method: "PUT",
        json: { summary },
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["summary", conversationId] });
    },
  });
}

export function useClearSummary(conversationId?: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<void>(`/api/conversations/${conversationId}/summary`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["summary", conversationId] });
    },
  });
}

export function useCompactNow(conversationId?: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<ConversationSummary>(`/api/conversations/${conversationId}/compact`, {
        method: "POST",
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["summary", conversationId] });
    },
  });
}
