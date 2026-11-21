import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import type { Conversation, ConversationSettings, SavedPrompt, SavedPromptsResponse } from "@/lib/types";

export interface ConversationFilters {
  pinned?: boolean;
  archived?: boolean;
}

export function useConversations(search = "", filters: ConversationFilters = {}, enabled = true) {
  const params = new URLSearchParams();
  if (search) params.set("q", search);
  if (filters.pinned) params.set("pinned", "true");
  if (filters.archived) params.set("archived", "true");
  const query = params.toString();
  return useQuery({
    queryKey: ["conversations", search, filters.pinned ?? false, filters.archived ?? false],
    queryFn: () => apiFetch<Conversation[]>(query ? `/api/conversations?${query}` : "/api/conversations"),
    enabled,
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
    onSuccess: () => {
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

export function usePinConversation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, pinned }: { id: string; pinned: boolean }) =>
      apiFetch<void>(`/api/conversations/${id}/pin`, { method: "PUT", json: { pinned } }),
    onSuccess: (_data, { id }) => {
      void queryClient.invalidateQueries({ queryKey: ["conversations"] });
      void queryClient.invalidateQueries({ queryKey: ["conversation", id] });
    },
  });
}

export function useArchiveConversation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) =>
      apiFetch<void>(`/api/conversations/${id}/archive`, { method: "PUT", json: { archived } }),
    onSuccess: (_data, { id }) => {
      void queryClient.invalidateQueries({ queryKey: ["conversations"] });
      void queryClient.invalidateQueries({ queryKey: ["conversation", id] });
    },
  });
}

export function useActivateBranch(conversationId: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (messageId: string) =>
      apiFetch<void>(`/api/conversations/${conversationId}/messages/${messageId}/activate`, { method: "POST" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["conversation", conversationId] });
    },
  });
}

export function useForkConversation(conversationId: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (messageId?: string) =>
      apiFetch<Conversation>(`/api/conversations/${conversationId}/fork`, {
        method: "POST",
        json: messageId ? { messageId } : {},
      }),
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

export function useSavedPrompts(enabled = true) {
  return useQuery({
    queryKey: ["prompts"],
    queryFn: async () => (await apiFetch<SavedPromptsResponse>("/api/prompts")).prompts,
    enabled,
  });
}

export function useCreateSavedPrompt() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { title: string; body: string }) =>
      apiFetch<SavedPrompt>("/api/prompts", { method: "POST", json: input }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["prompts"] });
    },
  });
}

export function useUpdateSavedPrompt() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, title, body }: { id: string; title?: string; body?: string }) =>
      apiFetch<SavedPrompt>(`/api/prompts/${id}`, { method: "PATCH", json: { title, body } }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["prompts"] });
    },
  });
}

export function useDeleteSavedPrompt() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/api/prompts/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["prompts"] });
    },
  });
}

export function useMessageFeedback(conversationId: string | null) {
  return useMutation({
    mutationFn: ({ messageId, rating }: { messageId: string; rating: "up" | "down" | null }) => {
      const path = `/api/conversations/${conversationId}/messages/${messageId}/feedback`;
      return rating
        ? apiFetch<void>(path, { method: "PUT", json: { rating } })
        : apiFetch<void>(path, { method: "DELETE" });
    },
  });
}
