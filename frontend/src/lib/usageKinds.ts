// Human labels for usage kinds (see internal/httpapi usage events).
// Unknown kinds fall back to the raw string so new backend kinds still render.
const KINDS: Record<string, { label: string; description: string }> = {
  chat: { label: "Chat", description: "Model generations" },
  embedding: { label: "Embedding", description: "Query and document embeddings" },
  rerank: { label: "Rerank", description: "LLM re-ordering of retrieved chunks" },
  compaction: { label: "Compaction", description: "History summarization" },
  web_search: { label: "Web Search", description: "Live web retrieval queries" },
  document_search: { label: "Doc Search", description: "Knowledge base retrieval queries" },
  tool: { label: "Tool", description: "Tool and sandbox executions" },
};

export function kindLabel(kind: string): string {
  return KINDS[kind]?.label ?? kind;
}

export function kindDescription(kind: string): string {
  return KINDS[kind]?.description ?? "";
}

export function formatEngineOrToolName(name: string): string {
  if (!name) return "";
  const lower = name.toLowerCase().trim();
  if (lower === "wigolo" || lower === "web_search") return "Web Search";
  if (lower === "code_runner" || lower === "sandbox" || lower === "code_sandbox") return "Code Sandbox";
  if (lower === "google_calendar") return "Google Calendar";
  if (lower === "remember") return "Memory Store";
  if (lower === "search_documents") return "Doc Retrieval";
  if (lower === "list_documents") return "Doc List";
  if (lower === "read_document") return "Doc Reader";
  if (lower.startsWith("mcp_")) {
    const parts = name.slice(4).split("_");
    return parts.map((p) => p.charAt(0).toUpperCase() + p.slice(1)).join(" ");
  }
  return name.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}
