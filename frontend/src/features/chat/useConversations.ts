import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import type { Conversation, ConversationSettings } from "@/lib/types";

export function useConversations() {
  return useQuery({
    queryKey: ["conversations"],
    queryFn: () => apiFetch<Conversation[]>("/api/conversations"),
  });
}

export function useCreateConversation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (settings?: ConversationSettings) => {
      const body = settings
        ? { model: settings.model, temperature: settings.temperature ?? undefined, systemPrompt: settings.systemPrompt, ragEnabled: settings.ragEnabled }
        : {};
      return apiFetch<Conversation>("/api/conversations", { method: "POST", json: body });
    },
    onSuccess: (conversation) => {
      queryClient.setQueryData<Conversation[]>(["conversations"], (old) =>
        old ? [conversation, ...old.filter((c) => c.id !== conversation.id)] : [conversation],
      );
      void queryClient.invalidateQueries({ queryKey: ["conversations"] });
    },
  });
}

export function useUpdateConversationSettings(id: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (settings: ConversationSettings) => {
      const body: Record<string, unknown> = { model: settings.model, systemPrompt: settings.systemPrompt, ragEnabled: settings.ragEnabled };
      if (settings.temperature === null) body.clearTemperature = true;
      else body.temperature = settings.temperature;
      return apiFetch<void>(`/api/conversations/${id}`, { method: "PATCH", json: body });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["conversation", id] });
      void queryClient.invalidateQueries({ queryKey: ["conversations"] });
    },
  });
}

export function useDeleteConversation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/api/conversations/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["conversations"] });
    },
  });
}

export function useDeleteMessage(convID: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (messageId: string) =>
      apiFetch<void>(`/api/conversations/${convID}/messages/${messageId}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["conversation", convID] });
      void queryClient.invalidateQueries({ queryKey: ["conversations"] });
    },
  });
}
