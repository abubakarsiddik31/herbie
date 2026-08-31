import type { ToolParam, UserTool } from "@/lib/types";
import type { ToolTemplate } from "./templates";

// Header values render masked; sending the mask back means "keep the stored
// secret" (backend PATCH semantics).
export const HEADER_MASK = "••••";

export interface HeaderRow {
  key: string;
  value: string;
}

export interface ToolForm {
  name: string;
  description: string;
  method: string;
  urlTemplate: string;
  params: ToolParam[];
  bodyTemplate: string;
  headers: HeaderRow[];
  requireApproval: boolean;
  enabled: boolean;
}

export const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"];
export const PARAM_TYPES = ["string", "number", "boolean"] as const;

const NAME_RE = /^[a-z][a-z0-9_]{0,63}$/;
const PARAM_NAME_RE = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/;
const PLACEHOLDER_RE = /\{\{([A-Za-z_][A-Za-z0-9_]*)\}\}/g;

export function emptyForm(): ToolForm {
  return {
    name: "",
    description: "",
    method: "GET",
    urlTemplate: "https://",
    params: [],
    bodyTemplate: "",
    headers: [],
    requireApproval: false,
    enabled: true,
  };
}

export function formFromTool(t: UserTool): ToolForm {
  return {
    name: t.name,
    description: t.description,
    method: t.method,
    urlTemplate: t.urlTemplate,
    params: t.params.map((p) => ({ ...p })),
    bodyTemplate: t.bodyTemplate,
    headers: Object.entries(t.headers).map(([key, value]) => ({ key, value })),
    requireApproval: t.requireApproval,
    enabled: t.enabled,
  };
}

export function formFromTemplate(t: ToolTemplate): ToolForm {
  return {
    name: t.tool.name,
    description: t.tool.description,
    method: t.tool.method,
    urlTemplate: t.tool.urlTemplate,
    params: t.tool.params.map((p) => ({ ...p })),
    bodyTemplate: t.tool.bodyTemplate,
    headers: Object.entries(t.tool.headers).map(([key, value]) => ({ key, value })),
    requireApproval: t.tool.requireApproval,
    enabled: true,
  };
}

export function formToPayload(form: ToolForm): Record<string, unknown> {
  return {
    name: form.name.trim(),
    description: form.description.trim(),
    method: form.method,
    urlTemplate: form.urlTemplate.trim(),
    params: form.params,
    bodyTemplate: form.bodyTemplate,
    headers: Object.fromEntries(
      form.headers.filter((h) => h.key.trim() !== "").map((h) => [h.key, h.value]),
    ),
    requireApproval: form.requireApproval,
    enabled: form.enabled,
  };
}

export interface FormErrors {
  name?: string;
  url?: string;
  params?: string;
  body?: string;
}

// Client-side mirror of the backend authoring rules so obvious mistakes surface
// inline before a round trip. The server re-validates and stays authoritative.
export function validateForm(form: ToolForm): FormErrors {
  const errors: FormErrors = {};
  const name = form.name.trim();
  if (!NAME_RE.test(name)) {
    errors.name = "Must start with a lowercase letter; only lowercase letters, digits and underscores (max 64).";
  }

  const byName = new Map<string, ToolParam>();
  for (const p of form.params) {
    if (!PARAM_NAME_RE.test(p.name)) {
      errors.params = `Parameter “${p.name || "(unnamed)"}”: use letters, digits and underscores, starting with a letter.`;
      break;
    }
    if (byName.has(p.name)) {
      errors.params = `Parameter “${p.name}” is declared twice.`;
      break;
    }
    byName.set(p.name, p);
  }
  if (errors.params) return errors;

  const placeholders = new Set<string>();
  for (const m of form.urlTemplate.matchAll(PLACEHOLDER_RE)) {
    placeholders.add(m[1]);
    const p = byName.get(m[1]);
    if (!p) {
      errors.url = `{{${m[1]}}} is used in the URL but not declared as a parameter.`;
    } else if (p.in !== "path") {
      errors.url = `{{${m[1]}}} is a query parameter — query params are appended automatically, not written into the URL.`;
    }
    if (errors.url) return errors;
  }
  for (const p of form.params) {
    if (p.in === "path" && !placeholders.has(p.name)) {
      errors.url = `Parameter “${p.name}” is marked as a path parameter but does not appear in the URL.`;
      return errors;
    }
  }

  const sample = form.urlTemplate.trim().replace(PLACEHOLDER_RE, "x");
  try {
    const u = new URL(sample);
    if (u.protocol !== "http:" && u.protocol !== "https:") throw new Error("scheme");
  } catch {
    errors.url = "Enter an absolute http(s) URL.";
  }
  if (errors.url) return errors;

  if (form.bodyTemplate.trim() !== "") {
    if (form.method === "GET" || form.method === "DELETE") {
      errors.body = "A body template needs POST, PUT or PATCH.";
      return errors;
    }
    for (const m of form.bodyTemplate.matchAll(PLACEHOLDER_RE)) {
      if (!byName.has(m[1])) {
        errors.body = `{{${m[1]}}} is used in the body but not declared as a parameter.`;
        return errors;
      }
    }
  }
  return errors;
}

// Splits a URL template into text and {{placeholder}} segments for the
// highlighted preview.
export interface TemplateSegment {
  text: string;
  placeholder?: string;
}

export function splitTemplate(tpl: string): TemplateSegment[] {
  const segments: TemplateSegment[] = [];
  let prev = 0;
  for (const m of tpl.matchAll(PLACEHOLDER_RE)) {
    const start = m.index ?? 0;
    if (start > prev) segments.push({ text: tpl.slice(prev, start) });
    segments.push({ text: m[0], placeholder: m[1] });
    prev = start + m[0].length;
  }
  if (prev < tpl.length) segments.push({ text: tpl.slice(prev) });
  return segments;
}
