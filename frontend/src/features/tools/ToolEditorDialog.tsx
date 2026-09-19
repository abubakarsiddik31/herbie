import { useMemo, useState } from "react";
import {
  AlertCircle,
  Code2,
  Info,
  KeyRound,
  Plus,
  Settings2,
  ShieldCheck,
  Trash2,
  Wrench,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";
import type { ToolParam, UserTool } from "@/lib/types";
import type { ToolTemplate } from "@/features/tools/templates";
import {
  extractUrlPlaceholders,
  HEADER_MASK,
  METHODS,
  PARAM_TYPES,
  splitTemplate,
  validateForm,
  type HeaderRow,
  type ToolForm,
} from "@/features/tools/toolForm";

const METHOD_BUTTON_STYLES: Record<string, { active: string; inactive: string }> = {
  GET: {
    active: "border-emerald-600 bg-emerald-600/15 text-emerald-700 dark:text-emerald-400 font-semibold shadow-xs",
    inactive: "text-muted-foreground hover:text-foreground hover:bg-muted/60",
  },
  POST: {
    active: "border-sky-600 bg-sky-600/15 text-sky-700 dark:text-sky-400 font-semibold shadow-xs",
    inactive: "text-muted-foreground hover:text-foreground hover:bg-muted/60",
  },
  PUT: {
    active: "border-amber-600 bg-amber-600/15 text-amber-700 dark:text-amber-400 font-semibold shadow-xs",
    inactive: "text-muted-foreground hover:text-foreground hover:bg-muted/60",
  },
  PATCH: {
    active: "border-violet-600 bg-violet-600/15 text-violet-700 dark:text-violet-400 font-semibold shadow-xs",
    inactive: "text-muted-foreground hover:text-foreground hover:bg-muted/60",
  },
  DELETE: {
    active: "border-rose-600 bg-rose-600/15 text-rose-700 dark:text-rose-400 font-semibold shadow-xs",
    inactive: "text-muted-foreground hover:text-foreground hover:bg-muted/60",
  },
};

const fieldCls =
  "w-full rounded-lg border border-input bg-background px-3 py-2 text-sm shadow-xs outline-none transition-colors placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[2px] focus-visible:ring-ring/40 disabled:opacity-50";
const selectCls =
  "h-8 rounded-lg border border-input bg-background px-2.5 text-xs shadow-xs outline-none transition-colors focus-visible:border-ring focus-visible:ring-[2px] focus-visible:ring-ring/40";

type EditorTab = "endpoint" | "params" | "headers" | "settings";

interface ToolEditorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  form: ToolForm;
  setForm: React.Dispatch<React.SetStateAction<ToolForm>>;
  editing: UserTool | null;
  basedOn: ToolTemplate | null;
  onSave: () => Promise<void>;
  saving: boolean;
}

export function ToolEditorDialog({
  open,
  onOpenChange,
  form,
  setForm,
  editing,
  basedOn,
  onSave,
  saving,
}: ToolEditorDialogProps) {
  const [activeTab, setActiveTab] = useState<EditorTab>("endpoint");
  const [attemptedSave, setAttemptedSave] = useState(false);

  // Derive validation errors reactively when save has been attempted
  const errors = useMemo(() => {
    return attemptedSave ? validateForm(form) : {};
  }, [form, attemptedSave]);

  // Extract placeholders from URL to check if any are missing from params
  const urlPlaceholders = useMemo(
    () => extractUrlPlaceholders(form.urlTemplate),
    [form.urlTemplate],
  );

  const missingPathParams = useMemo(() => {
    const existing = new Set(
      form.params.filter((p) => p.in === "path").map((p) => p.name),
    );
    return urlPlaceholders.filter((p) => !existing.has(p));
  }, [urlPlaceholders, form.params]);

  function autoAddAllMissingPathParams() {
    setForm((f) => {
      const existing = new Set(f.params.map((p) => p.name));
      const newOnes = missingPathParams
        .filter((name) => !existing.has(name))
        .map((name) => ({
          name,
          in: "path" as const,
          type: "string" as const,
          required: true,
          description: `${name} path parameter`,
        }));
      return {
        ...f,
        params: [...f.params, ...newOnes],
      };
    });
  }

  function setParam(index: number, patch: Partial<ToolParam>) {
    setForm((f) => ({
      ...f,
      params: f.params.map((p, idx) => (idx === index ? { ...p, ...patch } : p)),
    }));
  }

  function removeParam(index: number) {
    setForm((f) => ({
      ...f,
      params: f.params.filter((_, idx) => idx !== index),
    }));
  }

  function setHeader(index: number, patch: Partial<HeaderRow>) {
    setForm((f) => ({
      ...f,
      headers: f.headers.map((h, idx) => (idx === index ? { ...h, ...patch } : h)),
    }));
  }

  function removeHeader(index: number) {
    setForm((f) => ({
      ...f,
      headers: f.headers.filter((_, idx) => idx !== index),
    }));
  }

  function addHeaderPreset(key: string, value: string) {
    setForm((f) => {
      // Don't add duplicate keys
      if (f.headers.some((h) => h.key.toLowerCase() === key.toLowerCase())) {
        return f;
      }
      return {
        ...f,
        headers: [...f.headers, { key, value }],
      };
    });
  }

  async function handleSave() {
    setAttemptedSave(true);
    const foundErrors = validateForm(form);
    if (Object.keys(foundErrors).length > 0) {
      // Auto-switch to tab containing error
      if (foundErrors.name || foundErrors.url) {
        setActiveTab("endpoint");
      } else if (foundErrors.params || foundErrors.body) {
        setActiveTab("params");
      }
      return;
    }
    await onSave();
  }

  function handleDialogClose(nextOpen: boolean) {
    if (!nextOpen) {
      setAttemptedSave(false);
      setActiveTab("endpoint");
    }
    onOpenChange(nextOpen);
  }

  const hasEndpointErrors = Boolean(errors.name || errors.url);
  const hasParamErrors = Boolean(errors.params || errors.body);

  return (
    <Dialog open={open} onOpenChange={handleDialogClose}>
      <DialogContent className="flex max-h-[94dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl">
        {/* Modal Header */}
        <DialogHeader className="border-b px-6 py-4 text-left">
          <div className="flex items-center gap-2">
            <div className="flex size-8 items-center justify-center rounded-lg border bg-muted/60 text-foreground">
              <Wrench className="size-4" />
            </div>
            <div>
              <DialogTitle className="text-base font-semibold">
                {editing
                  ? `Edit ${editing.name}`
                  : basedOn
                    ? `New Tool: ${basedOn.title}`
                    : "Create Custom Tool"}
              </DialogTitle>
              <DialogDescription className="text-xs text-muted-foreground mt-0.5">
                {editing
                  ? "Changes take effect from your next chat interaction."
                  : basedOn
                    ? `Prefilled from ${basedOn.title} template. Adjust options as needed.`
                    : "Define an HTTP API with parameters that Herbie can call autonomously."}
              </DialogDescription>
            </div>
          </div>

          {/* Navigation Tabs */}
          <div className="mt-4 flex gap-1 border-b border-border/40 pt-1 -mb-4 overflow-x-auto [scrollbar-width:none]">
            <button
              type="button"
              onClick={() => setActiveTab("endpoint")}
              className={cn(
                "flex items-center gap-1.5 border-b-2 px-3.5 py-2 text-xs font-medium transition-colors shrink-0",
                activeTab === "endpoint"
                  ? "border-primary text-foreground font-semibold"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              <span>Endpoint & Basics</span>
              {hasEndpointErrors && (
                <span className="flex size-2 rounded-full bg-destructive" />
              )}
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("params")}
              className={cn(
                "flex items-center gap-1.5 border-b-2 px-3.5 py-2 text-xs font-medium transition-colors shrink-0",
                activeTab === "params"
                  ? "border-primary text-foreground font-semibold"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              <Code2 className="size-3.5" />
              <span>Parameters</span>
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono h-4">
                {form.params.length}
              </Badge>
              {hasParamErrors && (
                <span className="flex size-2 rounded-full bg-destructive" />
              )}
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("headers")}
              className={cn(
                "flex items-center gap-1.5 border-b-2 px-3.5 py-2 text-xs font-medium transition-colors shrink-0",
                activeTab === "headers"
                  ? "border-primary text-foreground font-semibold"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              <KeyRound className="size-3.5" />
              <span>Headers & Auth</span>
              {form.headers.length > 0 && (
                <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono h-4">
                  {form.headers.length}
                </Badge>
              )}
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("settings")}
              className={cn(
                "flex items-center gap-1.5 border-b-2 px-3.5 py-2 text-xs font-medium transition-colors shrink-0",
                activeTab === "settings"
                  ? "border-primary text-foreground font-semibold"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              <Settings2 className="size-3.5" />
              <span>Agent Behavior</span>
              {form.requireApproval && (
                <ShieldCheck className="size-3 text-amber-600 dark:text-amber-400" />
              )}
            </button>
          </div>
        </DialogHeader>

        {/* Tab Content Body */}
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          {/* TAB 1: ENDPOINT & BASICS */}
          {activeTab === "endpoint" && (
            <div className="space-y-5">
              <div className="grid gap-2">
                <div className="flex items-center justify-between">
                  <Label htmlFor="tool-name" className="text-xs font-medium">
                    Tool Identifier <span className="text-destructive">*</span>
                  </Label>
                  <span className="text-[11px] text-muted-foreground font-mono">
                    lowercase, digits, underscores
                  </span>
                </div>
                <Input
                  id="tool-name"
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  placeholder="e.g. get_crypto_price"
                  className="font-mono text-sm"
                  autoComplete="off"
                  spellCheck={false}
                />
                {errors.name ? (
                  <p className="flex items-center gap-1 text-xs text-destructive">
                    <AlertCircle className="size-3 shrink-0" /> {errors.name}
                  </p>
                ) : (
                  <p className="text-[11px] text-muted-foreground">
                    This is the unique function name that the AI model references when invoking this tool.
                  </p>
                )}
              </div>

              <div className="grid gap-2">
                <div className="flex items-center justify-between">
                  <Label htmlFor="tool-desc" className="text-xs font-medium">
                    Tool Description <span className="text-destructive">*</span>
                  </Label>
                  <span className="text-[11px] text-muted-foreground">Prompt guidance</span>
                </div>
                <textarea
                  id="tool-desc"
                  value={form.description}
                  onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
                  rows={3}
                  placeholder="Explain what this tool does and when the model should invoke it (e.g. 'Get the current price and 24-hour volume for any cryptocurrency')."
                  className={cn(fieldCls, "resize-none leading-relaxed text-xs")}
                />
                <p className="text-[11px] text-muted-foreground">
                  The LLM reads this description to decide when this tool should be selected over answering directly.
                </p>
              </div>

              {/* Method Selector */}
              <div className="grid gap-2 border-t pt-4">
                <Label className="text-xs font-medium">HTTP Method</Label>
                <div className="flex flex-wrap gap-1.5">
                  {METHODS.map((m) => {
                    const isSelected = form.method === m;
                    const style = METHOD_BUTTON_STYLES[m];
                    return (
                      <button
                        key={m}
                        type="button"
                        onClick={() => setForm((f) => ({ ...f, method: m }))}
                        className={cn(
                          "rounded-lg border px-3 py-1.5 text-xs font-mono transition-all",
                          isSelected
                            ? style?.active ?? "border-primary bg-primary/10 text-primary font-semibold"
                            : style?.inactive ?? "border-border text-muted-foreground hover:bg-muted",
                        )}
                      >
                        {m}
                      </button>
                    );
                  })}
                </div>
              </div>

              {/* URL Template */}
              <div className="grid gap-2">
                <div className="flex items-center justify-between">
                  <Label htmlFor="tool-url" className="text-xs font-medium">
                    URL Endpoint Template <span className="text-destructive">*</span>
                  </Label>
                  <span className="text-[11px] text-muted-foreground">
                    Path params use <code className="font-mono">{"{{name}}"}</code>
                  </span>
                </div>
                <Input
                  id="tool-url"
                  value={form.urlTemplate}
                  onChange={(e) => setForm((f) => ({ ...f, urlTemplate: e.target.value }))}
                  placeholder="https://api.example.com/v1/items/{{item_id}}"
                  className="font-mono text-xs"
                  autoComplete="off"
                  spellCheck={false}
                />
                {errors.url && (
                  <p className="flex items-center gap-1 text-xs text-destructive">
                    <AlertCircle className="size-3 shrink-0" /> {errors.url}
                  </p>
                )}

                {/* Path parameters detection helper banner */}
                {missingPathParams.length > 0 && (
                  <div className="flex items-center justify-between gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-2.5 text-xs text-amber-800 dark:text-amber-300">
                    <div className="flex items-center gap-2">
                      <Info className="size-4 shrink-0 text-amber-600 dark:text-amber-400" />
                      <span>
                        Found {missingPathParams.length} path variable{missingPathParams.length > 1 ? "s" : ""}:{" "}
                        {missingPathParams.map((p) => `{{${p}}}`).join(", ")}
                      </span>
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="xs"
                      onClick={autoAddAllMissingPathParams}
                      className="border-amber-500/40 bg-background text-foreground hover:bg-amber-500/20"
                    >
                      Add as Parameter{missingPathParams.length > 1 ? "s" : ""}
                    </Button>
                  </div>
                )}

                {/* Live Syntax Preview */}
                {form.urlTemplate.trim() !== "" && (
                  <div className="rounded-lg border bg-muted/30 p-3">
                    <p className="text-[10px] font-semibold tracking-wider text-muted-foreground uppercase">
                      URL Preview
                    </p>
                    <p className="mt-1 break-all font-mono text-xs leading-relaxed text-foreground">
                      {splitTemplate(form.urlTemplate).map((seg, i) =>
                        seg.placeholder ? (
                          <span
                            key={i}
                            className="rounded bg-amber-500/20 px-1 py-0.5 font-semibold text-amber-700 dark:text-amber-400"
                          >
                            {`{{${seg.placeholder}}}`}
                          </span>
                        ) : (
                          <span key={i}>{seg.text}</span>
                        ),
                      )}
                    </p>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* TAB 2: PARAMETERS & BODY */}
          {activeTab === "params" && (
            <div className="space-y-6">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-semibold">Tool Parameters ({form.params.length})</h3>
                  <p className="text-xs text-muted-foreground">
                    Define the arguments that Herbie should extract and pass to this endpoint.
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  {missingPathParams.length > 0 && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={autoAddAllMissingPathParams}
                      className="text-xs border-amber-500/40 text-amber-700 dark:text-amber-300"
                    >
                      <Plus className="size-3.5" /> Auto-add URL Params
                    </Button>
                  )}
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      setForm((f) => ({
                        ...f,
                        params: [
                          ...f.params,
                          {
                            name: "",
                            in: "query",
                            type: "string",
                            required: false,
                            description: "",
                          },
                        ],
                      }))
                    }
                  >
                    <Plus className="size-3.5" /> Add Parameter
                  </Button>
                </div>
              </div>

              {errors.params && (
                <div className="flex items-center gap-2 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
                  <AlertCircle className="size-4 shrink-0" />
                  <span>{errors.params}</span>
                </div>
              )}

              {form.params.length === 0 ? (
                <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-8 text-center">
                  <p className="text-xs font-medium text-foreground">No parameters defined</p>
                  <p className="mt-1 max-w-sm text-xs text-muted-foreground">
                    If this API accepts query parameters (like <code>?city=berlin</code>) or path variables (like <code>/item/{"{{id}}"}</code>), add them here.
                  </p>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="mt-3"
                    onClick={() =>
                      setForm((f) => ({
                        ...f,
                        params: [
                          ...f.params,
                          {
                            name: "",
                            in: "query",
                            type: "string",
                            required: false,
                            description: "",
                          },
                        ],
                      }))
                    }
                  >
                    <Plus className="size-3.5" /> Add First Parameter
                  </Button>
                </div>
              ) : (
                <div className="space-y-3">
                  {form.params.map((p, i) => {
                    const isPathParam = p.in === "path";
                    return (
                      <div
                        key={i}
                        className="rounded-xl border bg-card p-3.5 shadow-xs space-y-3 transition-colors hover:border-foreground/20"
                      >
                        <div className="flex flex-wrap items-center gap-2">
                          <div className="flex-1 min-w-[140px]">
                            <Input
                              value={p.name}
                              onChange={(e) => setParam(i, { name: e.target.value })}
                              placeholder="param_name"
                              aria-label={`Parameter ${i + 1} name`}
                              className="h-8 font-mono text-xs"
                              autoComplete="off"
                              spellCheck={false}
                            />
                          </div>

                          <div className="w-28">
                            <select
                              value={p.in}
                              onChange={(e) =>
                                setParam(i, { in: e.target.value as ToolParam["in"] })
                              }
                              aria-label={`Parameter ${i + 1} location`}
                              className={cn(selectCls, "w-full font-mono text-xs")}
                            >
                              <option value="query">query (?key=)</option>
                              <option value="path">path (/{"{{key}}"}/)</option>
                            </select>
                          </div>

                          <div className="w-24">
                            <select
                              value={p.type}
                              onChange={(e) =>
                                setParam(i, { type: e.target.value as ToolParam["type"] })
                              }
                              aria-label={`Parameter ${i + 1} type`}
                              className={cn(selectCls, "w-full font-mono text-xs")}
                            >
                              {PARAM_TYPES.map((tp) => (
                                <option key={tp} value={tp}>
                                  {tp}
                                </option>
                              ))}
                            </select>
                          </div>

                          <label className="flex items-center gap-1.5 cursor-pointer rounded-lg border bg-muted/40 px-2.5 py-1 text-xs text-muted-foreground select-none">
                            <input
                              type="checkbox"
                              checked={p.required}
                              onChange={(e) => setParam(i, { required: e.target.checked })}
                              aria-label={`Parameter ${i + 1} required`}
                              className="size-3.5 rounded accent-primary"
                            />
                            <span>Required</span>
                          </label>

                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-xs"
                            aria-label={`Remove parameter ${i + 1}`}
                            className="text-muted-foreground hover:text-destructive ml-auto"
                            onClick={() => removeParam(i)}
                          >
                            <Trash2 className="size-3.5" />
                          </Button>
                        </div>

                        <div className="flex items-center gap-2">
                          <Input
                            value={p.description}
                            onChange={(e) => setParam(i, { description: e.target.value })}
                            placeholder="Describe what this parameter is and any format requirements..."
                            aria-label={`Parameter ${i + 1} description`}
                            className="h-8 text-xs"
                          />
                        </div>

                        {isPathParam && (
                          <p className="text-[11px] text-amber-700 dark:text-amber-400 font-mono">
                            Must appear as <code className="font-semibold">{`{{${p.name || "name"}}}`}</code> in the URL template above.
                          </p>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}

              {/* Request Body Template (For POST / PUT / PATCH) */}
              {(form.method === "POST" || form.method === "PUT" || form.method === "PATCH") && (
                <div className="grid gap-2 border-t pt-5">
                  <div className="flex items-center justify-between">
                    <Label htmlFor="tool-body" className="text-xs font-medium">
                      JSON Request Body Template
                    </Label>
                    <span className="text-[11px] text-muted-foreground font-mono">
                      Substitutes <code className="font-semibold">{"{{param}}"}</code>
                    </span>
                  </div>
                  <textarea
                    id="tool-body"
                    value={form.bodyTemplate}
                    onChange={(e) => setForm((f) => ({ ...f, bodyTemplate: e.target.value }))}
                    rows={4}
                    placeholder={`{\n  "query": "{{query}}",\n  "limit": {{limit}}\n}`}
                    className={cn(fieldCls, "resize-none font-mono text-xs leading-relaxed")}
                    spellCheck={false}
                  />
                  {errors.body ? (
                    <p className="flex items-center gap-1 text-xs text-destructive">
                      <AlertCircle className="size-3 shrink-0" /> {errors.body}
                    </p>
                  ) : (
                    <p className="text-[11px] text-muted-foreground">
                      Sent as the JSON payload. Placeholders are replaced with parameter values before the request is dispatched.
                    </p>
                  )}
                </div>
              )}
            </div>
          )}

          {/* TAB 3: HEADERS & AUTH */}
          {activeTab === "headers" && (
            <div className="space-y-5">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <h3 className="text-sm font-semibold">Request Headers</h3>
                  <p className="text-xs text-muted-foreground">
                    Sent with every outgoing request. Sensitive values are masked and encrypted.
                  </p>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    setForm((f) => ({
                      ...f,
                      headers: [...f.headers, { key: "", value: "" }],
                    }))
                  }
                >
                  <Plus className="size-3.5" /> Add Header
                </Button>
              </div>

              {/* Quick Presets */}
              <div className="rounded-xl border bg-muted/20 p-3">
                <p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider mb-2">
                  Quick Presets
                </p>
                <div className="flex flex-wrap gap-1.5">
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    onClick={() => addHeaderPreset("Authorization", "Bearer <YOUR_TOKEN>")}
                    className="text-xs"
                  >
                    + Authorization: Bearer
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    onClick={() => addHeaderPreset("X-API-Key", "<YOUR_KEY>")}
                    className="text-xs"
                  >
                    + X-API-Key
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    onClick={() => addHeaderPreset("Accept", "application/json")}
                    className="text-xs"
                  >
                    + Accept: application/json
                  </Button>
                </div>
              </div>

              {form.headers.length === 0 ? (
                <div className="rounded-xl border border-dashed p-6 text-center text-xs text-muted-foreground">
                  No custom headers configured. If the API requires authentication or custom content types, add them above.
                </div>
              ) : (
                <div className="space-y-2">
                  {form.headers.map((h, i) => (
                    <div
                      key={i}
                      className="grid grid-cols-[1fr_1.5fr_auto] items-center gap-2 rounded-lg border bg-card p-2"
                    >
                      <Input
                        value={h.key}
                        onChange={(e) => setHeader(i, { key: e.target.value })}
                        placeholder="Header name (e.g. Authorization)"
                        aria-label={`Header ${i + 1} name`}
                        className="h-8 font-mono text-xs"
                        autoComplete="off"
                        spellCheck={false}
                      />
                      <Input
                        value={h.value}
                        onChange={(e) => setHeader(i, { value: e.target.value })}
                        placeholder={h.value === HEADER_MASK ? "Keep stored secret" : "Value or token"}
                        type={h.value === HEADER_MASK ? "password" : "text"}
                        aria-label={`Header ${i + 1} value`}
                        className="h-8 font-mono text-xs"
                        autoComplete="off"
                      />
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-xs"
                        aria-label={`Remove header ${i + 1}`}
                        className="text-muted-foreground hover:text-destructive"
                        onClick={() => removeHeader(i)}
                      >
                        <Trash2 className="size-3.5" />
                      </Button>
                    </div>
                  ))}
                </div>
              )}

              <div className="rounded-xl border border-primary/20 bg-primary/5 p-3.5 flex items-start gap-3">
                <Info className="size-4 shrink-0 text-primary mt-0.5" />
                <div className="text-xs leading-relaxed text-muted-foreground">
                  <p className="font-medium text-foreground">Secure Secret Storage</p>
                  <p className="mt-0.5">
                    Secrets are encrypted and stored exclusively in your backend database. They are never sent to the LLM and are masked in the UI with <code>••••</code>.
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* TAB 4: AGENT SETTINGS */}
          {activeTab === "settings" && (
            <div className="space-y-4">
              <div>
                <h3 className="text-sm font-semibold">Agent Execution Policy</h3>
                <p className="text-xs text-muted-foreground">
                  Configure how Herbie handles security and availability for this tool.
                </p>
              </div>

              <div className="divide-y overflow-hidden rounded-xl border bg-card">
                {/* Require Approval */}
                <div className="flex items-center justify-between gap-4 p-4">
                  <div className="space-y-0.5 min-w-0 pr-4">
                    <div className="flex items-center gap-1.5">
                      <ShieldCheck className="size-4 text-amber-600 dark:text-amber-400" />
                      <p className="text-sm font-medium">Require Approval Before Execution</p>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      Herbie will pause and ask you to confirm before invoking this tool in chat. Recommended for actions that modify records, send messages, or cost money.
                    </p>
                  </div>
                  <Switch
                    checked={form.requireApproval}
                    onCheckedChange={(v) => setForm((f) => ({ ...f, requireApproval: v }))}
                    aria-label="Toggle require approval"
                  />
                </div>

                {/* Enabled */}
                <div className="flex items-center justify-between gap-4 p-4">
                  <div className="space-y-0.5 min-w-0 pr-4">
                    <p className="text-sm font-medium">Enable Tool for Agent</p>
                    <p className="text-xs text-muted-foreground">
                      When enabled, this tool is registered into the agent’s system prompt for all chats. When disabled, the agent cannot see or call it.
                    </p>
                  </div>
                  <Switch
                    checked={form.enabled}
                    onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))}
                    aria-label="Toggle tool active"
                  />
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Modal Footer */}
        <DialogFooter className="border-t bg-muted/30 px-6 py-3.5 justify-between sm:justify-between">
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>

          <div className="flex items-center gap-3">
            {Object.keys(errors).length > 0 && (
              <span className="flex items-center gap-1 text-xs text-destructive">
                <AlertCircle className="size-3.5" /> Please review highlighted fields
              </span>
            )}
            <Button
              size="sm"
              variant="brand"
              onClick={() => void handleSave()}
              disabled={!form.name.trim() || !form.description.trim() || saving}
            >
              {saving ? "Saving Tool…" : editing ? "Save Changes" : "Create Tool"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
