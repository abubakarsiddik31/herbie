import { describe, expect, it, vi } from "vitest";
import type { ChatMessage, Conversation } from "@/lib/types";
import { conversationFilename, downloadMarkdown, toMarkdown } from "./exportMarkdown";

const conv: Conversation = {
  id: "c1", title: "Moon Facts", createdAt: "", updatedAt: "",
  model: "gemini-2.5-flash", temperature: null, systemPrompt: "", ragEnabled: true,
};

function msg(partial: Partial<ChatMessage> & { role: "user" | "assistant"; content: string }): ChatMessage {
  return { id: "m", truncated: false, createdAt: "2026-01-01T00:00:00Z", ...partial };
}

describe("toMarkdown", () => {
  it("renders turns, code fences, and a totals footer", () => {
    const md = toMarkdown(conv, [
      msg({ id: "m1", role: "user", content: "Show me code" }),
      msg({
        id: "m2", role: "assistant", content: "Here:\n```ts\nconst x = 1;\n```",
        usage: { inputTokens: 10, outputTokens: 20, costUsd: 0.00007, model: "gemini-2.5-flash" },
      }),
    ]);
    expect(md).toContain("# Moon Facts");
    expect(md).toContain("**User** · 2026-01-01T00:00:00Z");
    expect(md).toContain("```ts\nconst x = 1;\n```");
    expect(md).toContain("*Model: gemini-2.5-flash · 2 messages · 10 in / 20 out · $0.00007 · Exported from Herbie*");
  });

  it("marks images and truncated answers without breaking the text", () => {
    const md = toMarkdown(conv, [
      msg({
        id: "m1", role: "user", content: "What is this?",
        images: [{ mediaType: "image/png", dataUrl: "data:image/png;base64,xx" }],
      }),
      msg({ id: "m2", role: "assistant", content: "Partial", truncated: true }),
    ]);
    expect(md).toContain("> [image attachment: png]");
    expect(md).toContain("*(stopped early)*");
    expect(md).not.toContain("data:image/png");
  });

  it("falls back for untitled threads and default models", () => {
    const md = toMarkdown({ ...conv, title: "", model: "" }, []);
    expect(md.startsWith("# Untitled")).toBe(true);
    expect(md).toContain("Model: default · 0 messages");
  });
});

describe("conversationFilename", () => {
  it("slugifies the title and stamps the date", () => {
    expect(conversationFilename("Moon Facts!", new Date("2026-03-04T00:00:00Z"))).toBe(
      "herbie-moon-facts-2026-03-04.md",
    );
    expect(conversationFilename("!!!", new Date("2026-03-04T00:00:00Z"))).toBe(
      "herbie-untitled-2026-03-04.md",
    );
  });
});

describe("downloadMarkdown", () => {
  it("triggers an anchor download and releases the object URL", () => {
    const create = vi.fn().mockReturnValue("blob:url");
    const revoke = vi.fn();
    window.URL.createObjectURL = create;
    window.URL.revokeObjectURL = revoke;
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    downloadMarkdown("f.md", "# hi");
    expect(create).toHaveBeenCalledOnce();
    expect(click).toHaveBeenCalledOnce();
    const anchor = click.mock.instances[0] as HTMLAnchorElement;
    expect(anchor.download).toBe("f.md");
    expect(anchor.href).toBe("blob:url");
    expect(revoke).toHaveBeenCalledWith("blob:url");
    click.mockRestore();
  });
});
