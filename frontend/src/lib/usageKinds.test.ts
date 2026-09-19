import { describe, expect, it } from "vitest";
import { kindDescription, kindLabel } from "./usageKinds";

describe("usage kinds", () => {
  it("labels known kinds", () => {
    expect(kindLabel("chat")).toBe("Chat");
    expect(kindLabel("embedding")).toBe("Embedding");
    expect(kindLabel("rerank")).toBe("Rerank");
    expect(kindLabel("compaction")).toBe("Compaction");
    expect(kindLabel("web_search")).toBe("Web Search");
    expect(kindLabel("document_search")).toBe("Doc Search");
  });

  it("falls back to the raw kind", () => {
    expect(kindLabel("custom_unknown_tool")).toBe("custom_unknown_tool");
    expect(kindDescription("custom_unknown_tool")).toBe("");
  });

  it("describes the RAG kinds", () => {
    expect(kindDescription("rerank")).toMatch(/re-order/i);
    expect(kindDescription("compaction")).toMatch(/summar/i);
    expect(kindDescription("web_search")).toMatch(/web retrieval/i);
    expect(kindDescription("document_search")).toMatch(/knowledge base/i);
  });
});
