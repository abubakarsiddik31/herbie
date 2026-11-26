import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import type { CompareResponse } from "@/lib/types";

export function useCompare(conversationId?: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { content: string; models: string[]; clientMessageId?: string }) =>
      apiFetch<CompareResponse>(`/api/conversations/${conversationId}/compare`, {
        method: "POST",
        json: input,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["conversation", conversationId] });
      void queryClient.invalidateQueries({ queryKey: ["conversations"] });
    },
  });
}
