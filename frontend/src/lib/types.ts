export interface User { id: string; email: string }
export interface Conversation {
  id: string; title: string; createdAt: string; updatedAt: string;
  model: string; temperature: number | null; systemPrompt: string;
}
export interface ModelInfo { id: string; label: string; provider: string }
export interface ModelsResponse { default: string; models: ModelInfo[] }
export interface ConversationSettings { model: string; temperature: number | null; systemPrompt: string }
export interface MessageUsage { inputTokens: number; outputTokens: number; costUsd: number; model: string }
export interface ChatMessage {
  id: string; role: "user" | "assistant"; content: string;
  truncated: boolean; createdAt: string;
  streaming?: boolean; error?: string;
  images?: { mediaType: string; dataUrl: string }[];
  usage?: MessageUsage;
}
export interface UsageTotalsRow { kind: string; model: string; inputTokens: number; outputTokens: number; requests: number; costUsd: number }
export interface UsageDailyRow { day: string; inputTokens: number; outputTokens: number; costUsd: number }
export interface UsageSummary { totals: UsageTotalsRow[]; daily: UsageDailyRow[] }

export interface ToolParam {
  name: string;
  in: "path" | "query";
  type: "string" | "number" | "boolean";
  required: boolean;
  description: string;
}

export interface UserTool {
  id: string;
  name: string;
  description: string;
  method: string;
  urlTemplate: string;
  params: ToolParam[];
  bodyTemplate: string;
  headers: Record<string, string>;
  requireApproval: boolean;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface PendingApproval {
  callId: string;
  toolName: string;
  args: unknown;
  reason: string;
}
