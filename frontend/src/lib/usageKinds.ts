// Human labels for usage kinds (see internal/httpapi usage events).
// Unknown kinds fall back to the raw string so new backend kinds still render.
const KINDS: Record<string, { label: string; description: string }> = {
  chat: { label: "Chat", description: "Model generations" },
  embedding: { label: "Embedding", description: "Query and document embeddings" },
  rerank: { label: "Rerank", description: "LLM re-ordering of retrieved chunks" },
  compaction: { label: "Compaction", description: "History summarization" },
};

export function kindLabel(kind: string): string {
  return KINDS[kind]?.label ?? kind;
}

export function kindDescription(kind: string): string {
  return KINDS[kind]?.description ?? "";
}
