import { describe, expect, it } from "vitest";
import { cleanConversationTitle, fmtTokens, isFileTitle } from "./utils";

describe("fmtTokens", () => {
  it("renders small counts as-is", () => {
    expect(fmtTokens(0)).toBe("0");
    expect(fmtTokens(999)).toBe("999");
  });
  it("renders thousands compactly", () => {
    expect(fmtTokens(1200)).toBe("1.2k");
    expect(fmtTokens(45600)).toBe("45.6k");
  });
});

describe("cleanConversationTitle", () => {
  it("cleans raw file headers with prompt", () => {
    const raw = "--- File: 2026-financials.xlsx ---\n```xlsx\n[data]\n```\n\nAnalyze the EBITDA trends";
    expect(cleanConversationTitle(raw)).toBe("Analyze the EBITDA trends");
  });

  it("extracts filename when prompt is default or empty", () => {
    const raw = "--- File: report.pdf ---\n```pdf\n[binary]\n```\n\nPlease analyze the attached file(s) above.";
    expect(cleanConversationTitle(raw)).toBe("report.pdf");
  });

  it("formats multiple filenames cleanly", () => {
    const raw = "--- File: data.csv ---\n```csv\n```\n\n--- File: notes.txt ---\n```txt\n```\n\nPlease analyze the attached file(s) above.";
    expect(cleanConversationTitle(raw)).toBe("data.csv, notes.txt");
  });

  it("cleans trailing backticks and raw markdown headers", () => {
    expect(cleanConversationTitle("### Roadmap Review 2026")).toBe("Roadmap Review 2026");
    expect(cleanConversationTitle("--- File: script.py --- ```py")).toBe("script.py");
  });

  it("handles empty or null values gracefully", () => {
    expect(cleanConversationTitle("")).toBe("Untitled");
    expect(cleanConversationTitle(null)).toBe("Untitled");
    expect(cleanConversationTitle(undefined)).toBe("Untitled");
  });
});

describe("isFileTitle", () => {
  it("identifies file titles", () => {
    expect(isFileTitle("--- File: quarterly.pdf ---")).toBe(true);
    expect(isFileTitle("budget.xlsx")).toBe(true);
    expect(isFileTitle("main.go")).toBe(true);
    expect(isFileTitle("General Discussion")).toBe(false);
  });
});
