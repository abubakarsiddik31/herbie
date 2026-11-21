interface FindableMessage {
  id: string;
  content: string;
  quoteText?: string;
}

const MIN_FIND_CHARS = 2;

// In-conversation find matches the visible transcript only (content plus the
// quote snapshot). Case-insensitive substring matching keeps it predictable
// and instant; the server FTS covers cross-conversation search.
export function findMessageMatches(messages: FindableMessage[], query: string): string[] {
  const q = query.trim().toLowerCase();
  if (q.length < MIN_FIND_CHARS) return [];
  return messages
    .filter((m) => `${m.quoteText ?? ""}\n${m.content}`.toLowerCase().includes(q))
    .map((m) => m.id);
}
