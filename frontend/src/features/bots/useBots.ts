import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import type { Bot, Conversation } from "@/lib/types";

export interface BotMemory {
  id: string;
  content: string;
  createdAt: string;
}

export interface BotsResponse {
  bots: Bot[];
  count: number;
}

export interface BotDetail {
  bot: Bot;
  memories: BotMemory[];
}

/** Body for POST /api/bots; PATCH accepts the same fields, all optional. */
export interface BotInput {
  name: string;
  emoji: string;
  color: string;
  instructions: string;
  model: string;
  temperature: number | null;
  ragEnabled: boolean;
}

export function useBots() {
  return useQuery({
    queryKey: ["bots"],
    queryFn: () => apiFetch<BotsResponse>("/api/bots"),
  });
}

export function useBot(id: string | null) {
  return useQuery({
    queryKey: ["bot", id],
    queryFn: () => apiFetch<BotDetail>(`/api/bots/${id}`),
    enabled: id !== null,
  });
}

export function useCreateBot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: BotInput) =>
      apiFetch<{ bot: Bot }>("/api/bots", { method: "POST", json: body }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bots"] });
    },
  });
}

export function useUpdateBot(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: Partial<BotInput>) =>
      apiFetch<{ bot: Bot }>(`/api/bots/${id}`, { method: "PATCH", json: body }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bots"] });
      void queryClient.invalidateQueries({ queryKey: ["bot", id] });
    },
  });
}

export function useDeleteBot(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiFetch<void>(`/api/bots/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bots"] });
    },
  });
}

/** Starts (or resumes) the bot's conversation; the caller navigates to
 *  `/chat/{conversation.id}` with the returned conversation. */
export function useStartBotChat(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<{ conversation: Conversation }>(`/api/bots/${id}/conversations`, {
        method: "POST",
        json: {},
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bots"] });
      void queryClient.invalidateQueries({ queryKey: ["bot", id] });
      void queryClient.invalidateQueries({ queryKey: ["conversations"] });
    },
  });
}
