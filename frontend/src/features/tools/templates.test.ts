import { describe, expect, it } from "vitest";
import { TEMPLATES, templateHost, type ToolTemplateConfig } from "./templates";

// Mirrors backend/internal/chat/toolconfig.go Validate(). Every template must
// pass these rules, otherwise saving it straight from the gallery 400s.
const NAME_RE = /^[a-z][a-z0-9_]{0,63}$/;
const PARAM_NAME_RE = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/;
const PLACEHOLDER_RE = /\{\{([A-Za-z_][A-Za-z0-9_]*)\}\}/g;
const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"];

function firstValidationError(cfg: ToolTemplateConfig): string | null {
  if (!NAME_RE.test(cfg.name)) return `bad tool name ${cfg.name}`;
  if (cfg.description.length < 1 || cfg.description.length > 2000) return `bad description on ${cfg.name}`;
  if (!METHODS.includes(cfg.method)) return `bad method on ${cfg.name}`;

  const declared = new Map<string, { in: string }>();
  for (const p of cfg.params) {
    if (!PARAM_NAME_RE.test(p.name)) return `bad param name ${p.name} on ${cfg.name}`;
    if (declared.has(p.name)) return `duplicate param ${p.name} on ${cfg.name}`;
    if (p.in !== "path" && p.in !== "query") return `bad param location ${p.in} on ${cfg.name}`;
    if (p.type !== "string" && p.type !== "number" && p.type !== "boolean") {
      return `bad param type ${p.type} on ${cfg.name}`;
    }
    declared.set(p.name, { in: p.in });
  }

  // Placeholders in the URL must be declared path params, and every declared
  // path param must appear in the URL.
  const placeholders = new Set<string>();
  for (const m of cfg.urlTemplate.matchAll(PLACEHOLDER_RE)) {
    placeholders.add(m[1]);
    if (declared.get(m[1])?.in !== "path") return `placeholder {{${m[1]}}} on ${cfg.name} is not a declared path param`;
  }
  for (const p of cfg.params) {
    if (p.in === "path" && !placeholders.has(p.name)) {
      return `path param ${p.name} on ${cfg.name} missing from url template`;
    }
  }

  const sample = cfg.urlTemplate.replace(PLACEHOLDER_RE, "x");
  let parsed: URL;
  try {
    parsed = new URL(sample);
  } catch {
    return `unparsable url template on ${cfg.name}`;
  }
  if (parsed.protocol !== "https:" && parsed.protocol !== "http:") return `non-http(s) url on ${cfg.name}`;
  if (!parsed.hostname) return `missing host on ${cfg.name}`;

  if (cfg.bodyTemplate !== "" && !["POST", "PUT", "PATCH"].includes(cfg.method)) {
    return `body template on ${cfg.method} tool ${cfg.name}`;
  }
  if (cfg.params.length > 16) return `too many params on ${cfg.name}`;
  if (Object.keys(cfg.headers).length > 16) return `too many headers on ${cfg.name}`;
  return null;
}

describe("tool templates", () => {
  it("are numerous and span several categories", () => {
    expect(TEMPLATES.length).toBeGreaterThanOrEqual(10);
    expect(new Set(TEMPLATES.map((t) => t.category)).size).toBeGreaterThanOrEqual(5);
  });

  it("have unique slugs and unique tool names", () => {
    expect(new Set(TEMPLATES.map((t) => t.slug)).size).toBe(TEMPLATES.length);
    expect(new Set(TEMPLATES.map((t) => t.tool.name)).size).toBe(TEMPLATES.length);
  });

  it("all pass the backend authoring rules", () => {
    for (const t of TEMPLATES) {
      expect(firstValidationError(t.tool), `${t.slug}: ${t.tool.name}`).toBeNull();
    }
  });

  it("are read-only GET tools with approval off by default", () => {
    for (const t of TEMPLATES) {
      expect(t.tool.method).toBe("GET");
      expect(t.tool.requireApproval).toBe(false);
      expect(t.tool.enabled).toBe(true);
      expect(t.tool.bodyTemplate).toBe("");
    }
  });

  it("every template renders a display host", () => {
    for (const t of TEMPLATES) {
      expect(templateHost(t.tool.urlTemplate)).not.toContain("{{");
    }
  });

  it("all templates require zero authentication or API keys", () => {
    for (const t of TEMPLATES) {
      for (const [key] of Object.entries(t.tool.headers)) {
        expect(key.toLowerCase()).not.toContain("auth");
        expect(key.toLowerCase()).not.toContain("token");
        expect(key.toLowerCase()).not.toContain("key");
        expect(key.toLowerCase()).not.toContain("secret");
      }
    }
  });
});
