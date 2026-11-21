// The backend brackets matched terms with these private-use runes. Splitting
// on them lets the UI render <mark> text nodes without ever interpreting
// message content as HTML.
const HIT_START = "\uE000";
const HIT_STOP = "\uE001";

export interface SnippetPart {
  text: string;
  hit: boolean;
}

export function parseSnippet(snippet: string): SnippetPart[] {
  const parts: SnippetPart[] = [];
  let hit = false;
  let buffer = "";
  for (const ch of snippet) {
    if (ch === HIT_START || ch === HIT_STOP) {
      if (buffer) parts.push({ text: buffer, hit });
      buffer = "";
      hit = ch === HIT_START;
    } else {
      buffer += ch;
    }
  }
  if (buffer) parts.push({ text: buffer, hit });
  return parts;
}

export function HighlightedSnippet({ snippet }: { snippet: string }) {
  return (
    <>
      {parseSnippet(snippet).map((part, idx) =>
        part.hit ? (
          <mark key={idx} className="rounded-sm bg-primary/20 px-0.5 text-foreground">
            {part.text}
          </mark>
        ) : (
          <span key={idx}>{part.text}</span>
        ),
      )}
    </>
  );
}
