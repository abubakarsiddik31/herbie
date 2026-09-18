export interface User { id: string; email: string }
export interface Conversation {
  id: string; title: string; createdAt: string; updatedAt: string;
  model: string; temperature: number | null; systemPrompt: string; ragEnabled: boolean;
}
export interface ModelInfo { id: string; label: string; provider: string }
export interface ModelsResponse { default: string; models: ModelInfo[] }
export interface ConversationSettings { model: string; temperature: number | null; systemPrompt: string; ragEnabled: boolean }
export interface MessageUsage { inputTokens: number; outputTokens: number; costUsd: number; model: string }
export interface ChatMessage {
  id: string; role: "user" | "assistant"; content: string;
  truncated: boolean; createdAt: string;
  streaming?: boolean; error?: string;
  images?: { mediaType: string; dataUrl: string }[];
  usage?: MessageUsage;
  sources?: Source[];
}

export interface Source {
  documentId: string;
  title: string;
  heading: string;
  page: number;
  snippet: string;
  score: number;
}
export interface UsageTotalsRow { kind: string; model: string; inputTokens: number; outputTokens: number; requests: number; costUsd: number }
export interface UsageDailyRow { day: string; inputTokens: number; outputTokens: number; costUsd: number }
export interface UsageSummary { totals: UsageTotalsRow[]; daily: UsageDailyRow[]; documents?: UsageDocumentRow[] }

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

export interface DocumentRec {
  id: string;
  filename: string;
  mime: string;
  sizeBytes: number;
  status: "processing" | "ready" | "failed";
  error?: string;
  chunkCount: number;
  createdAt: string;
}

export interface UsageDocumentRow {
  documentId: string;
  filename: string;
  inputTokens: number;
  costUsd: number;
}
