import type { ChatMessage, Conversation } from "@/lib/types";

export function conversationFilename(title: string, now = new Date()): string {
  const slug =
    title
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "")
      .slice(0, 40) || "untitled";
  return `golem-${slug}-${now.toISOString().slice(0, 10)}.md`;
}

// toMarkdown renders a thread as a Markdown document. Message content
// passes through verbatim so code fences survive the round trip; image
// attachments become placeholder lines since their bytes stay in the app.
export function toMarkdown(conv: Conversation, messages: ChatMessage[]): string {
  const lines = [`# ${conv.title || "Untitled"}`, ""];
  let inTokens = 0;
  let outTokens = 0;
  let cost = 0;
  for (const m of messages) {
    if (m.role !== "user" && m.role !== "assistant") continue;
    lines.push(`**${m.role === "user" ? "User" : "Assistant"}** · ${m.createdAt}`, "");
    if (m.images) {
      for (const img of m.images) {
        lines.push(`> [image attachment: ${img.mediaType.replace("image/", "")}]`);
      }
      if (m.images.length > 0 && m.content) lines.push("");
    }
    if (m.content) lines.push(m.content, "");
    if (m.truncated) lines.push("*(stopped early)*", "");
    if (m.usage) {
      inTokens += m.usage.inputTokens;
      outTokens += m.usage.outputTokens;
      cost += m.usage.costUsd;
    }
  }
  lines.push(
    "---",
    `*Model: ${conv.model || "default"} · ${messages.length} messages · ` +
      `${inTokens} in / ${outTokens} out · $${cost.toFixed(5)} · Exported from Golem*`,
  );
  return lines.join("\n");
}

export function downloadMarkdown(filename: string, text: string): void {
  const url = URL.createObjectURL(new Blob([text], { type: "text/markdown" }));
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}
