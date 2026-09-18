import { describe, expect, it } from "vitest";
import { kindDescription, kindLabel } from "./usageKinds";

describe("usage kinds", () => {
  it("labels known kinds", () => {
    expect(kindLabel("chat")).toBe("Chat");
    expect(kindLabel("embedding")).toBe("Embedding");
    expect(kindLabel("rerank")).toBe("Rerank");
    expect(kindLabel("compaction")).toBe("Compaction");
  });

  it("falls back to the raw kind", () => {
    expect(kindLabel("web_search")).toBe("web_search");
    expect(kindDescription("web_search")).toBe("");
  });

  it("describes the RAG kinds", () => {
    expect(kindDescription("rerank")).toMatch(/re-order/i);
    expect(kindDescription("compaction")).toMatch(/summar/i);
  });
});
