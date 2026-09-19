import { apiFetch } from "./api";

export interface AttachedFile {
  id: string;
  name: string;
  size: number;
  type: string;
  content: string;
}

export const MAX_ATTACHED_FILES = 3;
export const MAX_FILE_SIZE_BYTES = 10 << 20; // 10 MB
export const MAX_ATTACHED_CHARS = 400_000; // ~100K tokens (4 chars per token)

export const BINARY_DOC_EXTENSIONS = new Set(["pdf", "docx", "xlsx", "pptx"]);

const TEXT_EXTENSIONS = new Set([
  "txt", "md", "markdown", "json", "csv", "tsv", "log",
  "py", "js", "jsx", "ts", "tsx", "go", "rs", "java", "c", "cpp", "h", "hpp",
  "cs", "rb", "php", "html", "css", "scss", "sass", "less",
  "yaml", "yml", "toml", "xml", "sh", "bash", "zsh", "sql", "env",
]);

export function isSupportedDocOrCodeFile(file: File): boolean {
  const ext = file.name.split(".").pop()?.toLowerCase() || "";
  if (BINARY_DOC_EXTENSIONS.has(ext)) return true;
  if (TEXT_EXTENSIONS.has(ext)) return true;
  if (file.type.startsWith("text/")) return true;
  return false;
}

export async function processAttachedFile(file: File): Promise<AttachedFile> {
  if (file.size > MAX_FILE_SIZE_BYTES) {
    throw new Error(`${file.name} exceeds maximum size of 10 MB`);
  }

  const ext = file.name.split(".").pop()?.toLowerCase() || "";

  let content: string;
  if (BINARY_DOC_EXTENSIONS.has(ext)) {
    const formData = new FormData();
    formData.append("file", file);
    const res = await apiFetch<{ filename: string; text: string; size: number }>("/api/extract-text", {
      method: "POST",
      body: formData,
    });
    content = res.text;
  } else {
    content = await file.text();
  }

  if (content.length > MAX_ATTACHED_CHARS) {
    throw new Error(`${file.name} exceeds 100K token limit (~${Math.round(content.length / 4).toLocaleString()} tokens). Please upload it to Projects or Documents for retrieval.`);
  }

  return {
    id: `${file.name}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    name: file.name,
    size: file.size,
    type: file.type || ext,
    content,
  };
}

export function formatPromptWithFiles(prompt: string, files: AttachedFile[]): string {
  if (!files || files.length === 0) return prompt;

  const parts: string[] = [];
  for (const f of files) {
    const ext = f.name.split(".").pop()?.toLowerCase() || "";
    parts.push(`--- File: ${f.name} ---\n\`\`\`${ext}\n${f.content}\n\`\`\``);
  }

  if (prompt.trim()) {
    parts.push(prompt.trim());
  } else {
    parts.push("Please analyze the attached file(s) above.");
  }

  return parts.join("\n\n");
}

export interface ExtractedFile {
  name: string;
  ext: string;
  content: string;
}

export function extractFilesAndPrompt(content: string): {
  filenames: string[];
  files: ExtractedFile[];
  userPrompt: string;
} {
  const fileRegex = /--- File: (.*?) ---\n```(\w*)\n([\s\S]*?)```\n*/g;
  const filenames: string[] = [];
  const files: ExtractedFile[] = [];
  let match: RegExpExecArray | null;
  while ((match = fileRegex.exec(content)) !== null) {
    filenames.push(match[1]);
    files.push({
      name: match[1],
      ext: match[2] || "",
      content: match[3],
    });
  }

  let userPrompt = content.replace(fileRegex, "").trim();
  if (userPrompt === "Please analyze the attached file(s) above.") {
    userPrompt = "";
  }
  return { filenames, files, userPrompt };
}
