export interface User { id: string; email: string; role?: "admin" | "user" | string }
export interface OAuthProvider { id: string; name: string }
export interface SharedThread { title: string; messages: ChatMessage[] }
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

export interface WorkflowNode {
  id: string;
  type: string;
  name: string;
  position: { x: number; y: number };
  data: Record<string, unknown>;
}

export interface WorkflowEdge {
  id: string;
  source: string;
  target: string;
  sourceHandle?: string;
  targetHandle?: string;
}

export interface Workflow {
  id: string;
  name: string;
  description: string;
  triggerType: string;
  webhookSlug?: string | null;
  webhookSecret?: string;
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  exposeAsTool: boolean;
  toolName: string;
  toolDescription: string;
  toolRequireApproval?: boolean;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface NodeExecutionResult {
  nodeId: string;
  nodeName: string;
  nodeType: string;
  status: "running" | "success" | "failed" | "skipped";
  input?: unknown;
  output?: unknown;
  error?: string;
  durationMs: number;
  startedAt?: string;
  finishedAt?: string;
}

export interface WorkflowRun {
  id: string;
  workflowId: string;
  status: "pending" | "running" | "success" | "failed";
  triggerSource: string;
  inputData: unknown;
  outputData: unknown;
  nodeResults: Record<string, NodeExecutionResult>;
  error?: string | null;
  durationMs: number;
  createdAt: string;
  finishedAt?: string | null;
}

export interface WorkflowCredential {
  id: string;
  name: string;
  type: string;
  provider?: string;
  scopes?: string[];
  expiresAt?: string | null;
  data: Record<string, string>;
  createdAt: string;
  updatedAt: string;
}

export interface ToolOAuthProvider {
  id: string;
  name: string;
  configured: boolean;
  connected: boolean;
  connectedVia?: "oauth" | "mcp" | "both";
  credentialName?: string;
  mcpServerId?: string;
  mcpServerName?: string;
  scopes?: string[];
  connectedAt?: string;
  expiresAt?: string | null;
}

export interface ToolAuditLog {
  id: string;
  userId: string;
  callerType: string;
  callerId: string;
  toolName: string;
  action: string;
  inputSummary: string;
  outputSummary: string;
  status: "success" | "failed" | "approval_pending" | "rejected" | "approved";
  error?: string | null;
  durationMs: number;
  createdAt: string;
}
