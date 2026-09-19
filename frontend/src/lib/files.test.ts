import { describe, expect, it } from "vitest";
import {
  extractFilesAndPrompt,
  formatPromptWithFiles,
  isSupportedDocOrCodeFile,
  MAX_ATTACHED_FILES,
  MAX_ATTACHED_CHARS,
} from "./files";

describe("files utility", () => {
  it("enforces max attached files and characters constants", () => {
    expect(MAX_ATTACHED_FILES).toBe(3);
    expect(MAX_ATTACHED_CHARS).toBe(400_000);
  });
  it("recognizes supported document and code files", () => {
    expect(isSupportedDocOrCodeFile(new File([], "app.py"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "doc.pdf"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "report.docx"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "sheet.xlsx"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "slides.pptx"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "notes.md"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "script.sh"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "data.csv"))).toBe(true);
    expect(isSupportedDocOrCodeFile(new File([], "data.tsv"))).toBe(true);
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

  it("extracts filenames and clean user prompt from formatted content", () => {
    const content = `--- File: sample.pdf ---
\`\`\`pdf
# Section 1
Detailed extracted text across paragraphs...

# Section 2
More lines of extracted document text...
\`\`\`

Can you give me a summary of this paper?`;

    const { filenames, userPrompt } = extractFilesAndPrompt(content);
    expect(filenames).toEqual(["sample.pdf"]);
    expect(userPrompt).toBe("Can you give me a summary of this paper?");
    expect(userPrompt).not.toContain("Detailed extracted text");
  });
});
