import { useState } from "react";
import { Link } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2, Wrench } from "lucide-react";
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
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import {
  useCreateTool,
  useDeleteTool,
  useTools,
  useUpdateTool,
} from "@/features/tools/useTools";

// Header values render masked; sending the mask back means "keep the stored
// secret" (backend PATCH semantics).
const HEADER_MASK = "••••";

interface HeaderRow {
  key: string;
  value: string;
}

interface ToolForm {
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

const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"];
const PARAM_TYPES = ["string", "number", "boolean"] as const;

function emptyForm(): ToolForm {
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

function formFromTool(t: UserTool): ToolForm {
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

function formToPayload(form: ToolForm): Record<string, unknown> {
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

export function ToolsPage() {
  const { data: tools, isLoading } = useTools();
  const createTool = useCreateTool();
  const updateTool = useUpdateTool();
  const deleteTool = useDeleteTool();
  const queryClient = useQueryClient();

  const [editing, setEditing] = useState<UserTool | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [form, setForm] = useState<ToolForm>(emptyForm());

  function openCreate() {
    setEditing(null);
    setForm(emptyForm());
    setDialogOpen(true);
  }

  function openEdit(t: UserTool) {
    setEditing(t);
    setForm(formFromTool(t));
    setDialogOpen(true);
  }

  function save() {
    const payload = formToPayload(form);
    const mutation = editing ? updateTool.mutateAsync({ id: editing.id, patch: payload }) : createTool.mutateAsync(payload);
    void mutation.then(() => {
      setDialogOpen(false);
      void queryClient.invalidateQueries({ queryKey: ["tools"] });
    });
  }

  function toggle(t: UserTool, field: "enabled" | "requireApproval") {
    updateTool.mutate({ id: t.id, patch: { [field]: !t[field] } });
  }

  function remove(t: UserTool) {
    if (!window.confirm(`Delete tool “${t.name}”? The agent will no longer be able to call it.`)) return;
    deleteTool.mutate(t.id);
  }

  function setParam(i: number, patch: Partial<ToolParam>) {
    setForm((f) => ({
      ...f,
      params: f.params.map((p, idx) => (idx === i ? { ...p, ...patch } : p)),
    }));
  }

  return (
    <div className="flex h-svh flex-col bg-background">
      <header className="flex items-center justify-between border-b px-4 py-2.5">
        <h1 className="flex items-center gap-2 text-sm font-medium">
          <Wrench className="size-4" /> Tools
        </h1>
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" asChild>
            <Link to="/">Chat</Link>
          </Button>
          <Button size="sm" onClick={openCreate}>
            <Plus /> New tool
          </Button>
        </div>
      </header>

      <ScrollArea className="min-h-0 flex-1">
        <div className="mx-auto w-full max-w-3xl space-y-3 px-4 py-6">
          {isLoading && [0, 1, 2].map((i) => <Skeleton key={i} className="h-24 w-full" />)}
          {!isLoading && tools?.length === 0 && (
            <div className="flex h-[50vh] flex-col items-center justify-center gap-3 text-center text-muted-foreground text-sm">
              <p>No tools yet.</p>
              <p className="max-w-md">
                A tool is an HTTP API the agent can call during a chat. Create one, describe when to
                use it, and ask for it in conversation — e.g. a weather API.
              </p>
              <Button variant="outline" size="sm" onClick={openCreate}>
                <Plus /> Create your first tool
              </Button>
            </div>
          )}
          {tools?.map((t) => (
            <div key={t.id} className="rounded-lg border p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline" className="font-mono text-xs">{t.method}</Badge>
                    <span className="truncate font-medium text-sm">{t.name}</span>
                    {t.requireApproval && (
                      <Badge variant="secondary" className="text-xs">needs approval</Badge>
                    )}
                    {!t.enabled && <Badge variant="outline" className="text-muted-foreground text-xs">disabled</Badge>}
                  </div>
                  <p className="mt-1 line-clamp-2 text-muted-foreground text-sm">{t.description}</p>
                  <p className="mt-1 truncate font-mono text-muted-foreground text-xs">{t.urlTemplate}</p>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    variant={t.enabled ? "secondary" : "outline"}
                    size="sm"
                    onClick={() => toggle(t, "enabled")}
                  >
                    {t.enabled ? "Enabled" : "Disabled"}
                  </Button>
                  <Button variant="ghost" size="icon-sm" aria-label={`Edit ${t.name}`} onClick={() => openEdit(t)}>
                    <Pencil />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Delete ${t.name}`}
                    className="text-destructive hover:text-destructive"
                    onClick={() => remove(t)}
                  >
                    <Trash2 />
                  </Button>
                </div>
              </div>
            </div>
          ))}
        </div>
      </ScrollArea>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{editing ? `Edit ${editing.name}` : "New tool"}</DialogTitle>
            <DialogDescription>
              Define an HTTP API the agent may call. Keep the description sharp — the model uses it
              to decide when to call the tool.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="grid grid-cols-[7rem_1fr] items-start gap-3">
              <Label htmlFor="tool-name" className="pt-2">Name</Label>
              <div>
                <Input
                  id="tool-name"
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  placeholder="get_weather"
                  className="font-mono"
                />
                <p className="mt-1 text-muted-foreground text-xs">lowercase letters, digits, underscores</p>
              </div>

              <Label htmlFor="tool-desc" className="pt-2">Description</Label>
              <Input
                id="tool-desc"
                value={form.description}
                onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
                placeholder="Get the current temperature for coordinates."
              />

              <Label htmlFor="tool-method" className="pt-2">Method</Label>
              <select
                id="tool-method"
                value={form.method}
                onChange={(e) => setForm((f) => ({ ...f, method: e.target.value }))}
                className="h-9 rounded-md border border-input bg-transparent px-3 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
              >
                {METHODS.map((m) => <option key={m} value={m}>{m}</option>)}
              </select>

              <Label htmlFor="tool-url" className="pt-2">URL</Label>
              <div>
                <Input
                  id="tool-url"
                  value={form.urlTemplate}
                  onChange={(e) => setForm((f) => ({ ...f, urlTemplate: e.target.value }))}
                  placeholder="https://api.example.com/v1/forecast"
                  className="font-mono"
                />
                <p className="mt-1 text-muted-foreground text-xs">
                  path params go inline as <code className="font-mono">{"{{name}}"}</code>
                </p>
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>Parameters</Label>
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
                  <Plus /> Add parameter
                </Button>
              </div>
              {form.params.map((p, i) => (
                <div key={i} className="grid grid-cols-[1fr_5rem_5rem_5rem_1fr_auto] items-center gap-2">
                  <Input
                    value={p.name}
                    onChange={(e) => setParam(i, { name: e.target.value })}
                    placeholder="name"
                    className="font-mono"
                  />
                  <select
                    value={p.in}
                    onChange={(e) => setParam(i, { in: e.target.value as ToolParam["in"] })}
                    className="h-9 rounded-md border border-input bg-transparent px-2 text-sm"
                  >
                    <option value="query">query</option>
                    <option value="path">path</option>
                  </select>
                  <select
                    value={p.type}
                    onChange={(e) => setParam(i, { type: e.target.value as ToolParam["type"] })}
                    className="h-9 rounded-md border border-input bg-transparent px-2 text-sm"
                  >
                    {PARAM_TYPES.map((tp) => <option key={tp} value={tp}>{tp}</option>)}
                  </select>
                  <label className="flex items-center gap-1 text-muted-foreground text-xs">
                    <input
                      type="checkbox"
                      checked={p.required}
                      onChange={(e) => setParam(i, { required: e.target.checked })}
                    />
                    required
                  </label>
                  <Input
                    value={p.description}
                    onChange={(e) => setParam(i, { description: e.target.value })}
                    placeholder="what it means"
                  />
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Remove parameter"
                    className="text-destructive hover:text-destructive"
                    onClick={() => setForm((f) => ({ ...f, params: f.params.filter((_, idx) => idx !== i) }))}
                  >
                    <Trash2 />
                  </Button>
                </div>
              ))}
            </div>

            {(form.method === "POST" || form.method === "PUT" || form.method === "PATCH") && (
              <div className="space-y-1">
                <Label htmlFor="tool-body">Body template (JSON)</Label>
                <textarea
                  id="tool-body"
                  value={form.bodyTemplate}
                  onChange={(e) => setForm((f) => ({ ...f, bodyTemplate: e.target.value }))}
                  rows={3}
                  placeholder={`{"lat": {{latitude}}}`}
                  className="max-h-48 w-full resize-none rounded-md border border-input bg-transparent px-3 py-2 font-mono text-sm shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
                />
              </div>
            )}

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>Headers</Label>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setForm((f) => ({ ...f, headers: [...f.headers, { key: "", value: "" }] }))}
                >
                  <Plus /> Add header
                </Button>
              </div>
              {form.headers.map((h, i) => (
                <div key={i} className="grid grid-cols-[1fr_1fr_auto] items-center gap-2">
                  <Input
                    value={h.key}
                    onChange={(e) =>
                      setForm((f) => ({
                        ...f,
                        headers: f.headers.map((row, idx) => (idx === i ? { ...row, key: e.target.value } : row)),
                      }))
                    }
                    placeholder="Authorization"
                    className="font-mono"
                  />
                  <Input
                    value={h.value}
                    onChange={(e) =>
                      setForm((f) => ({
                        ...f,
                        headers: f.headers.map((row, idx) => (idx === i ? { ...row, value: e.target.value } : row)),
                      }))
                    }
                    placeholder={h.value === HEADER_MASK ? "keep stored secret" : "Bearer …"}
                    type={h.value === HEADER_MASK ? "password" : "text"}
                    className="font-mono"
                  />
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Remove header"
                    className="text-destructive hover:text-destructive"
                    onClick={() => setForm((f) => ({ ...f, headers: f.headers.filter((_, idx) => idx !== i) }))}
                  >
                    <Trash2 />
                  </Button>
                </div>
              ))}
            </div>

            <div className="flex gap-6">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={form.requireApproval}
                  onChange={(e) => setForm((f) => ({ ...f, requireApproval: e.target.checked }))}
                />
                Ask before running
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
                />
                Enabled
              </label>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button
              onClick={save}
              disabled={!form.name.trim() || !form.description.trim() || createTool.isPending || updateTool.isPending}
            >
              {createTool.isPending || updateTool.isPending ? "Saving…" : "Save tool"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
