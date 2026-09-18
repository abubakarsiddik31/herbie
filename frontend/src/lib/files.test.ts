import { describe, expect, it } from "vitest";
import { formatPromptWithFiles, isSupportedDocOrCodeFile } from "./files";

describe("files utility", () => {
  it("recognizes supported document and code files", () => {
    expect(isSupportedDocOrCodeFile(new File([], "app.py"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "doc.pdf"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "notes.md"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "script.sh"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "data.csv"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "binary.exe"))).toBe(false);
  });

  it("formats prompt with files properly", () => {
    const files = [
      {
        id: "1",
        name: "test.py",
        size: 100,
        type: "text/x-python",
        content: "print('hello')",
      },
    ];
    const formatted = formatPromptWithFiles("Explain this code", files);
    expect(formatted).toContain("--- File: test.py ---");
    expect(formatted).toContain("print('hello')");
    expect(formatted).toContain("Explain this code");
  });

  it("handles empty user prompt with default analysis instruction", () => {
    const files = [
      {
        id: "1",
        name: "data.json",
        size: 50,
        type: "application/json",
        content: "{\"status\": \"ok\"}",
      },
    ];
    const formatted = formatPromptWithFiles("", files);
    expect(formatted).toContain("Please analyze the attached file(s) above.");
  });
});
