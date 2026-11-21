import { describe, expect, it } from "vitest";
import { findMessageMatches } from "./findInConversation";

const messages = [
  { id: "m1", content: "Postgres replication lag" },
  { id: "m2", content: "Unrelated" },
  { id: "m3", content: "Answer", quoteText: "replication question" },
];

describe("findMessageMatches", () => {
  it("matches content and quote snapshots case-insensitively", () => {
    expect(findMessageMatches(messages, "REPLIC")).toEqual(["m1", "m3"]);
  });

  it("ignores queries shorter than two characters", () => {
    expect(findMessageMatches(messages, " r ")).toEqual([]);
  });

  it("returns no matches for absent text", () => {
    expect(findMessageMatches(messages, "missing")).toEqual([]);
  });
});
