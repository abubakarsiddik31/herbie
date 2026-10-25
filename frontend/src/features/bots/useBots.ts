import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ApiError, apiFetch } from "@/lib/api";
import type {
  Bot,
  BotFileMeta,
  BotMemory,
  BotRoutine,
  BotRun,
  BotRunStatus,
  Conversation,
  PendingCall,
} from "@/lib/types";

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

export interface BotRunsResponse {
  runs: BotRun[];
}

export interface BotRunDetail {
  run: BotRun;
  /** Filled only while the run is parked on waiting_approval; empty otherwise. */
  pending: PendingCall[];
}

export interface ApprovalDecision {
  callId: string;
  approved: boolean;
  reason?: string;
}

const ACTIVE_RUN_STATUSES: readonly BotRunStatus[] = ["queued", "running", "waiting_approval"];

export function isRunActive(status: BotRunStatus): boolean {
  return ACTIVE_RUN_STATUSES.includes(status);
}

// How often the runs list refetches: every 3s while any run is still in
// flight, otherwise not at all (same shape as documents' pollIntervalFor).
export function botRunsPollInterval(runs: BotRun[] | undefined): number | false {
  return runs?.some((r) => isRunActive(r.status)) ? 3000 : false;
}

export function useBotRuns(botId: string | null) {
  return useQuery({
    queryKey: ["bot-runs", botId],
    queryFn: () => apiFetch<BotRunsResponse>(`/api/bots/${botId}/runs?limit=20`),
    enabled: botId !== null,
    refetchInterval: (query) => botRunsPollInterval(query.state.data?.runs),
  });
}

export function useBotRun(runId: string | null) {
  return useQuery({
    queryKey: ["bot-run", runId],
    queryFn: () => apiFetch<BotRunDetail>(`/api/bot-runs/${runId}`),
    enabled: runId !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.run.status;
      return status === "running" || status === "waiting_approval" ? 3000 : false;
    },
  });
}

/** Enqueues a background task. The server 409s when a run is already active
 *  or another approval is outstanding — its message is toasted either way. */
export function useEnqueueTask(botId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { task: string; conversationId?: string }) =>
      apiFetch<{ run: BotRun }>(`/api/bots/${botId}/tasks`, { method: "POST", json: body }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bot-runs", botId] });
    },
    onError: (err) =>
      toast.error(err instanceof ApiError ? err.message : "Failed to start task"),
  });
}

export function useCancelRun(botId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (runId: string) =>
      apiFetch<{ run: BotRun }>(`/api/bot-runs/${runId}/cancel`, { method: "POST" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bot-runs", botId] });
    },
  });
}

export function useResolveRunApprovals(botId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: { runId: string; decisions: ApprovalDecision[] }) =>
      apiFetch<{ run: BotRun }>(`/api/bot-runs/${vars.runId}/approvals`, {
        method: "POST",
        json: { decisions: vars.decisions },
      }),
    onSuccess: (_data, vars) => {
      void queryClient.invalidateQueries({ queryKey: ["bot-runs", botId] });
      void queryClient.invalidateQueries({ queryKey: ["bot-run", vars.runId] });
    },
  });
}

export interface BotRoutinesResponse {
  routines: BotRoutine[];
}

/** Body for POST /api/bots/{id}/routines; PATCH also accepts `enabled`. */
export interface RoutineInput {
  name: string;
  schedule: string;
  timezone: string;
  prompt: string;
}

export function useBotRoutines(botId: string | null) {
  return useQuery({
    queryKey: ["bot-routines", botId],
    queryFn: () => apiFetch<BotRoutinesResponse>(`/api/bots/${botId}/routines`),
    enabled: botId !== null,
  });
}

export function useCreateRoutine(botId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: RoutineInput) =>
      apiFetch<{ routine: BotRoutine }>(`/api/bots/${botId}/routines`, {
        method: "POST",
        json: body,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bot-routines", botId] });
    },
  });
}

export function useUpdateRoutine(botId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: { routineId: string } & Partial<RoutineInput> & { enabled?: boolean }) =>
      apiFetch<{ routine: BotRoutine }>(`/api/bots/routines/${vars.routineId}`, {
        method: "PATCH",
        json: {
          ...(vars.name !== undefined && { name: vars.name }),
          ...(vars.schedule !== undefined && { schedule: vars.schedule }),
          ...(vars.timezone !== undefined && { timezone: vars.timezone }),
          ...(vars.prompt !== undefined && { prompt: vars.prompt }),
          ...(vars.enabled !== undefined && { enabled: vars.enabled }),
        },
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bot-routines", botId] });
    },
  });
}

export function useDeleteRoutine(botId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (routineId: string) =>
      apiFetch<void>(`/api/bots/routines/${routineId}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bot-routines", botId] });
    },
  });
}

/** Enqueues a manual run of the routine now; the run itself shows up in the
 *  Runs tab via the existing run polling. */
export function useRunRoutineNow(botId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (routineId: string) =>
      apiFetch<{ run: BotRun }>(`/api/bots/routines/${routineId}/run`, { method: "POST" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bot-routines", botId] });
      void queryClient.invalidateQueries({ queryKey: ["bot-runs", botId] });
    },
  });
}

export interface BotFilesResponse {
  files: BotFileMeta[];
}

export interface BotMemoriesResponse {
  memories: BotMemory[];
}

/** Workspace file listing (metadata only — no content endpoint exists). */
export function useBotFiles(botId: string | null) {
  return useQuery({
    queryKey: ["bot-files", botId],
    queryFn: () => apiFetch<BotFilesResponse>(`/api/bots/${botId}/files`),
    enabled: botId !== null,
  });
}

export function useBotMemories(botId: string | null) {
  return useQuery({
    queryKey: ["bot-memories", botId],
    queryFn: () => apiFetch<BotMemoriesResponse>(`/api/bots/${botId}/memories`),
    enabled: botId !== null,
  });
}

export function useDeleteBotMemory(botId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (memoryId: string) =>
      apiFetch<void>(`/api/bots/${botId}/memories/${memoryId}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["bot-memories", botId] });
    },
  });
}
