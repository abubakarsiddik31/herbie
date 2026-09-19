import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// fmtTokens renders token counts compactly: 1200 -> "1.2k".
export function fmtTokens(n: number): string {
  if (n < 1000) return String(n);
  return `${(n / 1000).toFixed(1)}k`;
}

/**
 * Strips raw file wrappers (e.g. "--- File: foo.pdf ---", code fences, or auto-file info)
 * and formats the conversation title cleanly for presentation in the sidebar and header.
 */
export function cleanConversationTitle(title?: string | null): string {
  if (!title || !title.trim()) return "Untitled";

  let raw = title.trim();

  // Check if title has the file header wrapper: --- File: filename.ext ---
  const fileHeaderPattern = /--- File:\s*([^\n\r]+?)\s*---(?:\s*```[\s\S]*?```)?/g;
  const matches = [...raw.matchAll(/--- File:\s*([^\n\r]+?)\s*---/g)];
  const fileNames = matches.map((m) => m[1]?.trim()).filter(Boolean);

  if (fileNames.length > 0) {
    // Remove the file header and any enclosed or trailing code fence
    let promptRemainder = raw.replace(fileHeaderPattern, "").trim();
    // Strip trailing incomplete code fences like ```py or ```
    promptRemainder = promptRemainder.replace(/```[\w]*\s*$/g, "").trim();

    const isDefaultPrompt =
      !promptRemainder ||
      promptRemainder === "Please analyze the attached file(s) above." ||
      promptRemainder === "Please analyze the attached file(s) above" ||
      promptRemainder === "Please analyze the attached file above." ||
      promptRemainder === "Please analyze the attached file above";

    if (!isDefaultPrompt) {
      raw = promptRemainder;
    } else {
      if (fileNames.length === 1) {
        return fileNames[0];
      }
      if (fileNames.length === 2) {
        return `${fileNames[0]}, ${fileNames[1]}`;
      }
      return `${fileNames[0]} + ${fileNames.length - 1} files`;
    }
  }

  // Clean up any lingering markdown prefixes, fences, or extra whitespace
  raw = raw
    .replace(/^```[\w]*\n?/gm, "")
    .replace(/```$/gm, "")
    .replace(/^[#*\->_\s]+/g, "")
    .replace(/\s+/g, " ")
    .trim();

  return raw || "Untitled";
}

/**
 * Returns true if the title or conversation represents an attached file / document.
 */
export function isFileTitle(title?: string | null): boolean {
  if (!title) return false;
  if (title.includes("--- File:") || title.startsWith("File:")) return true;
  const cleaned = cleanConversationTitle(title).toLowerCase();
  const fileExtensions = [
    ".pdf", ".docx", ".xlsx", ".pptx", ".csv", ".tsv", ".json",
    ".txt", ".md", ".py", ".js", ".ts", ".tsx", ".jsx", ".go", ".rs",
    ".html", ".css", ".png", ".jpg", ".jpeg", ".webp"
  ];
  return fileExtensions.some((ext) => cleaned.endsWith(ext));
}
