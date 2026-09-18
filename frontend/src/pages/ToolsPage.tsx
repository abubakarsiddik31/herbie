import { useState } from "react";
import { toast } from "sonner";
import { Pencil, Plus, ShieldCheck, Sidebar, Trash2, TriangleAlert, Wrench } from "lucide-react";
import { useSidebar } from "@/components/layout/SidebarContext";
import { cn } from "@/lib/utils";
import type { ToolParam, UserTool } from "@/lib/types";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  TEMPLATES,
  TEMPLATE_CATEGORIES,
  templateHost,
  type ToolTemplate,
} from "@/features/tools/templates";
import {
  emptyForm,
  formFromTemplate,
  formFromTool,
  formToPayload,
  HEADER_MASK,
  splitTemplate,
  validateForm,
  type FormErrors,
  type HeaderRow,
  type ToolForm,
} from "@/features/tools/toolForm";
import {
  useCreateTool,
  useDeleteTool,
  useTools,
  useUpdateTool,
} from "@/features/tools/useTools";

const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"];
const PARAM_TYPES = ["string", "number", "boolean"] as const;

// Semantic method coding — desaturated tints so the grid scans at a glance
// without turning into a color wall.
const METHOD_STYLES: Record<string, string> = {
  GET: "border-emerald-600/25 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400",
  POST: "border-sky-600/25 bg-sky-600/10 text-sky-700 dark:text-sky-400",
  PUT: "border-amber-600/25 bg-amber-600/10 text-amber-700 dark:text-amber-400",
  PATCH: "border-violet-600/25 bg-violet-600/10 text-violet-700 dark:text-violet-400",
  DELETE: "border-rose-600/25 bg-rose-600/10 text-rose-700 dark:text-rose-400",
};

const fieldCls =
  "w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:opacity-50";
const selectCls =
  "h-9 rounded-md border border-input bg-transparent px-3 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50";
const selectClsSm =
  "h-8 rounded-md border border-input bg-transparent px-2 text-xs shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50";

const enter = "animate-in fade-in slide-in-from-bottom-2 duration-300 [animation-fill-mode:backwards] motion-reduce:animate-none";

function UrlPreview({ template }: { template: string }) {
  if (template.trim() === "") return null;
  return (
    <div className="rounded-lg border bg-muted/40 px-3 py-2.5">
      <p className="text-[11px] font-medium tracking-wider text-muted-foreground uppercase">Preview</p>
      <p className="mt-1 break-all font-mono text-xs leading-relaxed">
        {splitTemplate(template).map((seg, i) =>
          seg.placeholder ? (
            <span
              key={i}
              className="rounded-sm bg-amber-500/15 px-0.5 font-semibold text-amber-700 dark:text-amber-400"
            >
              {`{{${seg.placeholder}}}`}
            </span>
          ) : (
            <span key={i}>{seg.text}</span>
          ),
        )}
      </p>
    </div>
  );
}

function TemplateCard({ template, index, onPick }: { template: ToolTemplate; index: number; onPick: () => void }) {
  const Icon = template.icon;
  return (
    <button
      type="button"
      onClick={onPick}
      style={{ animationDelay: `${Math.min(index, 10) * 35}ms` }}
      className={cn(
        "group w-56 shrink-0 snap-start rounded-xl border bg-card p-4 text-left shadow-xs transition-all duration-200",
        "hover:-translate-y-0.5 hover:border-foreground/25 hover:shadow-md hover:shadow-black/5",
        "active:translate-y-0 active:scale-[0.99]",
        "focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
        enter,
      )}
    >
      <div className="flex items-start justify-between">
        <span className="flex size-9 items-center justify-center rounded-lg border bg-muted/60 text-foreground/80">
          <Icon className="size-4" />
        </span>
        <Plus className="size-4 text-muted-foreground/30 transition-all duration-200 group-hover:translate-x-0.5 group-hover:text-foreground" />
      </div>
      <p className="mt-3 text-sm font-semibold tracking-tight">{template.title}</p>
      <p className="mt-1 line-clamp-2 min-h-8 text-xs leading-relaxed text-muted-foreground">{template.tagline}</p>
      <p className="mt-2 truncate font-mono text-[11px] text-muted-foreground/70">
        {templateHost(template.tool.urlTemplate)}
      </p>
    </button>
  );
}

function ToolCard({
  tool,
  index,
  onEdit,
  onDelete,
  onToggle,
}: {
  tool: UserTool;
  index: number;
  onEdit: () => void;
  onDelete: () => void;
  onToggle: (enabled: boolean) => void;
}) {
  return (
    <div
      style={{ animationDelay: `${Math.min(index, 8) * 40}ms` }}
      className={cn(
        "group relative flex flex-col rounded-xl border bg-card p-4 shadow-xs transition-all duration-200",
        "hover:-translate-y-0.5 hover:shadow-md hover:shadow-black/5",
        !tool.enabled && "opacity-70 saturate-[0.85]",
        enter,
      )}
    >
      <div className="flex items-center gap-2">
        <span
          className={cn(
            "shrink-0 rounded-md border px-1.5 py-0.5 font-mono text-[11px] font-semibold tracking-wide",
            METHOD_STYLES[tool.method] ?? "border-border bg-muted text-muted-foreground",
          )}
        >
          {tool.method}
        </span>
        <span className="truncate font-mono text-sm font-semibold">{tool.name}</span>
        <div className="ml-auto flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100 max-md:opacity-100">
          <Button variant="ghost" size="icon-xs" aria-label={`Edit ${tool.name}`} onClick={onEdit}>
            <Pencil />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={`Delete ${tool.name}`}
            className="text-muted-foreground hover:text-destructive"
            onClick={onDelete}
          >
            <Trash2 />
          </Button>
        </div>
      </div>

      <p className="mt-2 line-clamp-2 min-h-10 text-sm leading-snug text-muted-foreground">{tool.description}</p>

      <div className="mt-2 flex items-center gap-1.5 text-xs text-muted-foreground/80">
        <span className="truncate font-mono">{templateHost(tool.urlTemplate)}</span>
        <span className="shrink-0 text-muted-foreground/50">·</span>
        <span className="shrink-0">
          {tool.params.length === 0 ? "no params" : `${tool.params.length} param${tool.params.length === 1 ? "" : "s"}`}
        </span>
      </div>

      <div className="mt-3 flex items-center justify-between gap-2 border-t pt-3">
        <div className="flex min-w-0 items-center gap-1.5">
          {tool.requireApproval && (
            <Badge
              variant="outline"
              className="gap-1 border-amber-500/40 bg-amber-500/10 font-normal text-amber-700 dark:text-amber-400"
            >
              <ShieldCheck className="size-3" /> approval
            </Badge>
          )}
          {!tool.enabled && (
            <Badge variant="outline" className="font-normal text-muted-foreground">hidden from agent</Badge>
          )}
        </div>
        <label className="flex shrink-0 cursor-pointer items-center gap-2 text-xs text-muted-foreground">
          <Switch checked={tool.enabled} onCheckedChange={onToggle} aria-label={`Toggle ${tool.name}`} />
          {tool.enabled ? "Enabled" : "Disabled"}
        </label>
      </div>
    </div>
  );
}

export function ToolsPage() {
  const { toggleSidebar, setMobileOpen } = useSidebar();
  const { data: tools, isLoading, isError, refetch } = useTools();
  const createTool = useCreateTool();
  const updateTool = useUpdateTool();
  const deleteTool = useDeleteTool();

  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<UserTool | null>(null);
  const [basedOn, setBasedOn] = useState<ToolTemplate | null>(null);
  const [form, setForm] = useState<ToolForm>(emptyForm());
  const [errors, setErrors] = useState<FormErrors>({});
  const [deleting, setDeleting] = useState<UserTool | null>(null);
  const [category, setCategory] = useState<string>("All");

  const visibleTemplates = category === "All" ? TEMPLATES : TEMPLATES.filter((t) => t.category === category);
  const saving = createTool.isPending || updateTool.isPending;

  function openEditor(next: ToolForm, template: ToolTemplate | null, editingTool: UserTool | null) {
    setForm(next);
    setBasedOn(template);
    setEditing(editingTool);
    setErrors({});
    setEditorOpen(true);
  }

  function setParam(i: number, patch: Partial<ToolParam>) {
    setForm((f) => ({
      ...f,
      params: f.params.map((p, idx) => (idx === i ? { ...p, ...patch } : p)),
    }));
  }

  function setHeader(i: number, patch: Partial<HeaderRow>) {
    setForm((f) => ({
      ...f,
      headers: f.headers.map((row, idx) => (idx === i ? { ...row, ...patch } : row)),
    }));
  }

  async function save() {
    const found = validateForm(form);
    if (Object.keys(found).length > 0) {
      setErrors(found);
      return;
    }
    const payload = formToPayload(form);
    try {
      if (editing) {
        await updateTool.mutateAsync({ id: editing.id, patch: payload });
      } else {
        await createTool.mutateAsync(payload);
      }
      toast.success(editing ? "Tool updated" : `Tool “${form.name.trim()}” saved`);
      setEditorOpen(false);
    } catch {
      // the mutation hook already surfaced the API error as a toast
    }
  }

  function confirmDelete() {
    if (!deleting) return;
    const id = deleting.id;
    deleteTool.mutate(id, {
      onSuccess: () => {
        setDeleting(null);
        toast.success("Tool deleted");
      },
    });
  }

  return (
    <div className="flex h-full flex-col bg-background">
      <header className="flex items-center justify-between gap-2 border-b border-border/60 px-4 py-2 bg-background/80 backdrop-blur-xs z-10 shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => {
              if (window.innerWidth < 768) {
                setMobileOpen(true);
              } else {
                toggleSidebar();
              }
            }}
            aria-label="Toggle sidebar"
            title="Toggle sidebar (⌘B)"
            className="text-muted-foreground hover:text-foreground"
          >
            <Sidebar className="size-4" />
          </Button>
          <div className="flex items-center gap-2">
            <Wrench className="size-4 shrink-0 text-muted-foreground" />
            <h1 className="text-sm font-semibold tracking-tight">Tools</h1>
            {tools && (
              <Badge variant="secondary" className="shrink-0 font-mono text-xs px-1.5 py-0 h-4">
                {tools.length}
              </Badge>
            )}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Button size="sm" onClick={() => openEditor(emptyForm(), null, null)}>
            <Plus className="size-3.5" /> New tool
          </Button>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-5xl space-y-10 px-4 py-8">
          <section>
            <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
              <div>
                <h2 className="text-base font-semibold tracking-tight">Start from a template</h2>
                <p className="mt-0.5 text-sm text-muted-foreground">
                  Prebuilt tools on free, open APIs — pick one, tweak it, save.
                </p>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {["All", ...TEMPLATE_CATEGORIES].map((cat) => (
                  <button
                    key={cat}
                    type="button"
                    onClick={() => setCategory(cat)}
                    className={cn(
                      "rounded-full border px-3 py-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
                      category === cat
                        ? "border-transparent bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                    )}
                  >
                    {cat}
                  </button>
                ))}
              </div>
            </div>
            <div
              key={category}
              className="-mx-1 flex snap-x snap-mandatory gap-3 overflow-x-auto px-1 pb-1 [scrollbar-width:thin]"
            >
              {visibleTemplates.map((t, i) => (
                <TemplateCard
                  key={t.slug}
                  template={t}
                  index={i}
                  onPick={() => openEditor(formFromTemplate(t), t, null)}
                />
              ))}
            </div>
          </section>

          <section>
            <div className="mb-4 flex items-end justify-between gap-3">
              <div>
                <h2 className="text-base font-semibold tracking-tight">Your tools</h2>
                <p className="mt-0.5 text-sm text-muted-foreground">
                  Enabled tools are available to the agent in every chat.
                </p>
              </div>
              {tools && tools.length > 0 && (
                <span className="shrink-0 text-sm text-muted-foreground">
                  {tools.length} total
                </span>
              )}
            </div>

            {isLoading && (
              <div className="grid gap-3 sm:grid-cols-2">
                {[0, 1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-40 rounded-xl" />
                ))}
              </div>
            )}

            {isError && (
              <div className={cn("flex flex-col items-center gap-3 rounded-xl border border-destructive/30 bg-destructive/5 px-6 py-12 text-center", enter)}>
                <TriangleAlert className="size-5 text-destructive" />
                <p className="text-sm text-muted-foreground">Couldn’t load your tools.</p>
                <Button variant="outline" size="sm" onClick={() => void refetch()}>
                  Try again
                </Button>
              </div>
            )}

            {!isLoading && !isError && tools?.length === 0 && (
              <div className={cn("flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-14 text-center", enter)}>
                <span className="flex size-12 items-center justify-center rounded-full bg-muted">
                  <Wrench className="size-5 text-muted-foreground" />
                </span>
                <div>
                  <p className="font-medium">No tools yet</p>
                  <p className="mx-auto mt-1 max-w-sm text-sm text-muted-foreground">
                    Tools are HTTP APIs Golem can call mid-conversation. Pick a template above, or define
                    your own from scratch.
                  </p>
                </div>
                <Button size="sm" variant="outline" onClick={() => openEditor(emptyForm(), null, null)}>
                  <Plus /> Define a custom tool
                </Button>
              </div>
            )}

            {tools && tools.length > 0 && (
              <div className="grid gap-3 sm:grid-cols-2">
                {tools.map((t, i) => (
                  <ToolCard
                    key={t.id}
                    tool={t}
                    index={i}
                    onEdit={() => openEditor(formFromTool(t), null, t)}
                    onDelete={() => setDeleting(t)}
                    onToggle={(enabled) => updateTool.mutate({ id: t.id, patch: { enabled } })}
                  />
                ))}
              </div>
            )}
          </section>
        </div>
      </div>

      <Dialog open={editorOpen} onOpenChange={(open) => { if (!open) setEditorOpen(false); }}>
        <DialogContent className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
          <DialogHeader className="border-b px-6 py-4 text-left">
            <DialogTitle className="text-base">
              {editing ? `Edit ${editing.name}` : basedOn ? basedOn.title : "New tool"}
            </DialogTitle>
            <DialogDescription>
              {editing
                ? "Changes apply from the next chat turn on."
                : basedOn
                  ? `Prefilled from the ${basedOn.title} template — adjust anything before saving.`
                  : "Define an HTTP API the agent can call during chat."}
            </DialogDescription>
          </DialogHeader>

          <div className="min-h-0 flex-1 space-y-6 overflow-y-auto px-6 py-5">
            <section className="space-y-4">
              <div className="grid gap-2">
                <Label htmlFor="tool-name">Name</Label>
                <Input
                  id="tool-name"
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  placeholder="get_weather"
                  className="font-mono"
                  autoComplete="off"
                  spellCheck={false}
                />
                {errors.name ? (
                  <p className="text-xs text-destructive">{errors.name}</p>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    Lowercase letters, digits and underscores. This is the name the model sees.
                  </p>
                )}
              </div>
              <div className="grid gap-2">
                <Label htmlFor="tool-desc">Description</Label>
                <textarea
                  id="tool-desc"
                  value={form.description}
                  onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
                  rows={2}
                  placeholder="Get the current temperature and wind for coordinates."
                  className={cn(fieldCls, "min-h-0 resize-none")}
                />
                <p className="text-xs text-muted-foreground">
                  The model reads this to decide when to call the tool — say what it does and when to use it.
                </p>
              </div>
            </section>

            <section className="space-y-3 border-t pt-5">
              <div className="grid gap-3 sm:grid-cols-[7rem_1fr]">
                <div className="grid gap-2">
                  <Label htmlFor="tool-method">Method</Label>
                  <select
                    id="tool-method"
                    value={form.method}
                    onChange={(e) => setForm((f) => ({ ...f, method: e.target.value }))}
                    className={selectCls}
                  >
                    {METHODS.map((m) => <option key={m} value={m}>{m}</option>)}
                  </select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="tool-url">URL template</Label>
                  <Input
                    id="tool-url"
                    value={form.urlTemplate}
                    onChange={(e) => setForm((f) => ({ ...f, urlTemplate: e.target.value }))}
                    placeholder="https://api.example.com/v1/forecast"
                    className="font-mono"
                    autoComplete="off"
                    spellCheck={false}
                  />
                  {errors.url ? (
                    <p className="text-xs text-destructive">{errors.url}</p>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      Path params go inline as <code className="font-mono">{"{{name}}"}</code>; query params
                      are appended automatically.
                    </p>
                  )}
                </div>
              </div>
              <UrlPreview template={form.urlTemplate} />
            </section>

            <section className="space-y-3 border-t pt-5">
              <div className="flex items-center justify-between gap-2">
                <div>
                  <h3 className="text-sm font-semibold">Parameters</h3>
                  <p className="text-xs text-muted-foreground">Arguments the model fills in for each call.</p>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    setForm((f) => ({
                      ...f,
                      params: [...f.params, { name: "", in: "query", type: "string", required: false, description: "" }],
                    }))
                  }
                >
                  <Plus /> Add
                </Button>
              </div>
              {errors.params && <p className="text-xs text-destructive">{errors.params}</p>}
              {form.params.length === 0 && !errors.params && (
                <p className="rounded-lg border border-dashed px-3 py-3 text-center text-xs text-muted-foreground">
                  No parameters — the tool takes no arguments.
                </p>
              )}
              {form.params.length > 0 && (
                <div className="overflow-hidden rounded-lg border">
                  <div className="hidden grid-cols-[1.1fr_4.5rem_5rem_4rem_1.4fr_2rem] gap-2 border-b bg-muted/40 px-3 py-2 text-[11px] font-medium tracking-wider text-muted-foreground uppercase sm:grid">
                    <span>Name</span>
                    <span>In</span>
                    <span>Type</span>
                    <span className="text-center">Req</span>
                    <span>Description</span>
                    <span />
                  </div>
                  <div className="divide-y">
                    {form.params.map((p, i) => (
                      <div
                        key={i}
                        className="grid gap-2 px-3 py-2 sm:grid-cols-[1.1fr_4.5rem_5rem_4rem_1.4fr_2rem] sm:items-center"
                      >
                        <Input
                          value={p.name}
                          onChange={(e) => setParam(i, { name: e.target.value })}
                          placeholder="city"
                          aria-label={`Parameter ${i + 1} name`}
                          className="h-8 font-mono text-xs"
                          autoComplete="off"
                          spellCheck={false}
                        />
                        <select
                          value={p.in}
                          onChange={(e) => setParam(i, { in: e.target.value as ToolParam["in"] })}
                          aria-label={`Parameter ${i + 1} location`}
                          className={selectClsSm}
                        >
                          <option value="query">query</option>
                          <option value="path">path</option>
                        </select>
                        <select
                          value={p.type}
                          onChange={(e) => setParam(i, { type: e.target.value as ToolParam["type"] })}
                          aria-label={`Parameter ${i + 1} type`}
                          className={selectClsSm}
                        >
                          {PARAM_TYPES.map((tp) => <option key={tp} value={tp}>{tp}</option>)}
                        </select>
                        <label className="flex items-center gap-1.5 text-xs text-muted-foreground sm:justify-center">
                          <input
                            type="checkbox"
                            checked={p.required}
                            onChange={(e) => setParam(i, { required: e.target.checked })}
                            aria-label={`Parameter ${i + 1} required`}
                            className="size-3.5 accent-[var(--primary)]"
                          />
                          <span className="sm:hidden">required</span>
                        </label>
                        <Input
                          value={p.description}
                          onChange={(e) => setParam(i, { description: e.target.value })}
                          placeholder="What it means"
                          aria-label={`Parameter ${i + 1} description`}
                          className="h-8 text-xs"
                        />
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          aria-label={`Remove parameter ${i + 1}`}
                          className="justify-self-end text-muted-foreground hover:text-destructive"
                          onClick={() => setForm((f) => ({ ...f, params: f.params.filter((_, idx) => idx !== i) }))}
                        >
                          <Trash2 />
                        </Button>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </section>

            {(form.method === "POST" || form.method === "PUT" || form.method === "PATCH") && (
              <section className="space-y-2 border-t pt-5">
                <Label htmlFor="tool-body">Body template</Label>
                <textarea
                  id="tool-body"
                  value={form.bodyTemplate}
                  onChange={(e) => setForm((f) => ({ ...f, bodyTemplate: e.target.value }))}
                  rows={3}
                  placeholder={`{"city": {{city}}}`}
                  className={cn(fieldCls, "max-h-48 resize-none font-mono text-xs")}
                  spellCheck={false}
                />
                {errors.body ? (
                  <p className="text-xs text-destructive">{errors.body}</p>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    JSON sent with the request; <code className="font-mono">{"{{placeholders}}"}</code> are
                    substituted from the parameters.
                  </p>
                )}
              </section>
            )}

            <section className="space-y-3 border-t pt-5">
              <div className="flex items-center justify-between gap-2">
                <div>
                  <h3 className="text-sm font-semibold">Headers</h3>
                  <p className="text-xs text-muted-foreground">
                    Sent with every call. Values are masked after save.
                  </p>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setForm((f) => ({ ...f, headers: [...f.headers, { key: "", value: "" }] }))}
                >
                  <Plus /> Add
                </Button>
              </div>
              {form.headers.length === 0 && (
                <p className="rounded-lg border border-dashed px-3 py-3 text-center text-xs text-muted-foreground">
                  No headers — add e.g. <code className="font-mono">Authorization</code> for APIs that need a key.
                </p>
              )}
              {form.headers.length > 0 && (
                <div className="space-y-2">
                  {form.headers.map((h, i) => (
                    <div key={i} className="grid grid-cols-[1fr_1.4fr_2rem] items-center gap-2">
                      <Input
                        value={h.key}
                        onChange={(e) => setHeader(i, { key: e.target.value })}
                        placeholder="Authorization"
                        aria-label={`Header ${i + 1} name`}
                        className="h-8 font-mono text-xs"
                        autoComplete="off"
                        spellCheck={false}
                      />
                      <Input
                        value={h.value}
                        onChange={(e) => setHeader(i, { value: e.target.value })}
                        placeholder={h.value === HEADER_MASK ? "keep stored secret" : "Bearer …"}
                        type={h.value === HEADER_MASK ? "password" : "text"}
                        aria-label={`Header ${i + 1} value`}
                        className="h-8 font-mono text-xs"
                        autoComplete="off"
                      />
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        aria-label={`Remove header ${i + 1}`}
                        className="justify-self-end text-muted-foreground hover:text-destructive"
                        onClick={() => setForm((f) => ({ ...f, headers: f.headers.filter((_, idx) => idx !== i) }))}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  ))}
                </div>
              )}
              <p className="text-xs text-muted-foreground">
                Secrets are stored server-side, never sent to the model, and shown masked — resending the
                mask keeps the stored value.
              </p>
            </section>

            <section className="space-y-3 border-t pt-5">
              <h3 className="text-sm font-semibold">Behavior</h3>
              <div className="divide-y overflow-hidden rounded-lg border">
                <div className="flex items-center justify-between gap-4 px-3 py-3">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">Ask before running</p>
                    <p className="text-xs text-muted-foreground">
                      Golem pauses for your approval every time the model calls this tool.
                    </p>
                  </div>
                  <Switch
                    checked={form.requireApproval}
                    onCheckedChange={(v) => setForm((f) => ({ ...f, requireApproval: v }))}
                  />
                </div>
                <div className="flex items-center justify-between gap-4 px-3 py-3">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">Enabled</p>
                    <p className="text-xs text-muted-foreground">Disabled tools are hidden from the agent.</p>
                  </div>
                  <Switch
                    checked={form.enabled}
                    onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))}
                  />
                </div>
              </div>
            </section>
          </div>

          <DialogFooter className="border-t bg-muted/30 px-6 py-3.5">
            <Button variant="outline" size="sm" onClick={() => setEditorOpen(false)}>Cancel</Button>
            <Button
              size="sm"
              onClick={() => void save()}
              disabled={!form.name.trim() || !form.description.trim() || saving}
            >
              {saving ? "Saving…" : "Save tool"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={deleting !== null} onOpenChange={(open) => { if (!open) setDeleting(null); }}>
        <DialogContent showCloseButton={false} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete tool?</DialogTitle>
            <DialogDescription>
              “{deleting?.name}” will be removed and the agent will no longer be able to call it.
              Conversations that used it keep their history.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>Cancel</Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={deleteTool.isPending}>
              {deleteTool.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
