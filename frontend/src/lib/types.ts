export interface User { id: string; email: string }
export interface Conversation { id: string; title: string; createdAt: string; updatedAt: string }
export interface ChatMessage {
  id: string; role: "user" | "assistant"; content: string;
  truncated: boolean; createdAt: string;
  streaming?: boolean; error?: string;
}
export interface UsageTotalsRow { kind: string; model: string; inputTokens: number; outputTokens: number; requests: number; costUsd: number }
export interface UsageDailyRow { day: string; inputTokens: number; outputTokens: number; costUsd: number }
export interface UsageSummary { totals: UsageTotalsRow[]; daily: UsageDailyRow[] }
