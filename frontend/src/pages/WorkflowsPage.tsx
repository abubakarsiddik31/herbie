import { useState } from "react";
import { useNavigate } from "react-router";
import {
  Globe,
  Plus,
  Search,
  Sparkles,
  Trash2,
  Webhook,
  Workflow as WorkflowIcon,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
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
  useCreateWorkflow,
  useDeleteWorkflow,
  useUpdateWorkflow,
  useWorkflows,
} from "@/features/workflows/useWorkflows";
import type { Workflow } from "@/lib/types";

const WORKFLOW_TEMPLATES = [
  {
    id: "blank",
    title: "Blank Workflow",
    description: "Start with an empty canvas",
    icon: WorkflowIcon,
    nodes: [{ id: "start", type: "manual", name: "Manual Trigger", position: { x: 100, y: 150 }, data: {} }],
    edges: [],
  },
  {
    id: "webhook_slack",
    title: "Webhook to Slack Alert",
    description: "Receive webhooks and forward formatted alerts to Slack",
    icon: Webhook,
    nodes: [
      { id: "w1", type: "webhook", name: "Webhook", position: { x: 80, y: 150 }, data: {} },
      {
        id: "t1",
        type: "code_transform",
        name: "Format Alert",
        position: { x: 320, y: 150 },
        data: { fields: { text: "Incoming alert: {{ $json.body.message }}" } },
      },
      { id: "s1", type: "slack", name: "Send to Slack", position: { x: 560, y: 150 }, data: {} },
    ],
    edges: [
      { id: "e1", source: "w1", target: "t1" },
      { id: "e2", source: "t1", target: "s1" },
    ],
  },
  {
    id: "ai_summary",
    title: "HTTP Fetch & AI Summary",
    description: "Call an external API, summarize with Gemini, and return output",
    icon: Sparkles,
    nodes: [
      { id: "m1", type: "manual", name: "Start", position: { x: 80, y: 150 }, data: {} },
      {
        id: "h1",
        type: "http_request",
        name: "Fetch API",
        position: { x: 320, y: 150 },
        data: { url: "https://api.github.com/repos/golang/go", method: "GET" },
      },
      {
        id: "ai1",
        type: "llm_prompt",
        name: "Summarize Details",
        position: { x: 560, y: 150 },
        data: { prompt: "Summarize this repo info in 2 sentences:\n{{ $json.data.description }}" },
      },
    ],
    edges: [
      { id: "e1", source: "m1", target: "h1" },
      { id: "e2", source: "h1", target: "ai1" },
    ],
  },
];

export function WorkflowsPage() {
  const navigate = useNavigate();
  const { data: workflows, isLoading } = useWorkflows();
  const createWorkflow = useCreateWorkflow();
  const updateWorkflow = useUpdateWorkflow();
  const deleteWorkflow = useDeleteWorkflow();

  const [search, setSearch] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedTemplate, setSelectedTemplate] = useState("blank");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [pendingDelete, setPendingDelete] = useState<Workflow | null>(null);

  const filtered = (workflows || []).filter((w) => {
    const q = search.toLowerCase().trim();
    return !q || w.name.toLowerCase().includes(q) || w.description.toLowerCase().includes(q);
  });

  function handleCreate() {
    if (!name.trim()) {
      toast.error("Please provide a name for the workflow");
      return;
    }

    const tpl = WORKFLOW_TEMPLATES.find((t) => t.id === selectedTemplate) || WORKFLOW_TEMPLATES[0];

    createWorkflow.mutate(
      {
        name: name.trim(),
        description: description.trim(),
        triggerType: tpl.nodes[0]?.type || "manual",
        nodes: tpl.nodes as any,
        edges: tpl.edges as any,
        isActive: true,
      },
      {
        onSuccess: (created) => {
          toast.success("Workflow created");
          setCreateOpen(false);
          setName("");
          setDescription("");
          navigate(`/workflows/${created.id}`);
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : "Failed to create workflow");
        },
      }
    );
  }

  function handleToggleActive(e: React.MouseEvent, w: Workflow) {
    e.stopPropagation();
    updateWorkflow.mutate(
      { id: w.id, patch: { isActive: !w.isActive } },
      {
        onSuccess: () => {
          toast.success(!w.isActive ? "Workflow activated" : "Workflow deactivated");
        },
      }
    );
  }

  function confirmDelete() {
    if (!pendingDelete) return;
    deleteWorkflow.mutate(pendingDelete.id, {
      onSuccess: () => {
        toast.success("Workflow deleted");
        setPendingDelete(null);
      },
      onError: (err) => {
        toast.error(err instanceof Error ? err.message : "Failed to delete workflow");
      },
    });
  }

  return (
    <div className="flex flex-1 flex-col h-full bg-background text-foreground overflow-y-auto">
      <div className="max-w-6xl mx-auto w-full p-6 space-y-6">
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b border-border pb-5">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground flex items-center gap-2.5">
              <WorkflowIcon className="size-6 text-primary" />
              <span>Workflows</span>
            </h1>
            <p className="text-xs text-muted-foreground mt-1">
              Build n8n-style visual automation pipelines with AI, webhooks, and external tool integrations.
            </p>
          </div>
          <Button onClick={() => setCreateOpen(true)} className="gap-2 self-start sm:self-auto shadow-xs">
            <Plus className="size-4" />
            <span>New Workflow</span>
          </Button>
        </div>

        {/* Search */}
        <div className="relative max-w-md">
          <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search workflows by name or description..."
            className="pl-9 h-9 text-xs"
          />
        </div>

        {/* Workflow Cards */}
        {isLoading ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-44 w-full rounded-xl" />
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-border p-12 text-center">
            <div className="mx-auto flex size-12 items-center justify-center rounded-xl bg-primary/10 text-primary mb-3">
              <WorkflowIcon className="size-6" />
            </div>
            <h3 className="text-sm font-semibold text-foreground">No workflows found</h3>
            <p className="text-xs text-muted-foreground mt-1 max-w-sm mx-auto">
              {search ? "No workflows match your search query." : "Create your first workflow to automate tasks with external tools and AI."}
            </p>
            {!search && (
              <Button onClick={() => setCreateOpen(true)} className="mt-4 gap-2 text-xs" size="sm">
                <Plus className="size-3.5" />
                <span>Create Workflow</span>
              </Button>
            )}
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {filtered.map((w) => {
              const nodeCount = w.nodes?.length || 0;
              return (
                <Card
                  key={w.id}
                  onClick={() => navigate(`/workflows/${w.id}`)}
                  className="group relative flex flex-col justify-between p-4 transition-all hover:border-primary/50 hover:shadow-md cursor-pointer border border-border"
                >
                  <div>
                    <div className="flex items-start justify-between gap-2 mb-2">
                      <div className="flex items-center gap-2">
                        <span
                          className={`size-2 rounded-full shrink-0 ${
                            w.isActive ? "bg-emerald-500" : "bg-muted-foreground/40"
                          }`}
                        />
                        <h3 className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors line-clamp-1">
                          {w.name}
                        </h3>
                      </div>
                      <div className="flex items-center gap-1 shrink-0" onClick={(e) => e.stopPropagation()}>
                        <Switch
                          checked={w.isActive}
                          onClick={(e) => handleToggleActive(e, w)}
                          aria-label="Toggle active status"
                        />
                      </div>
                    </div>

                    <p className="text-xs text-muted-foreground line-clamp-2 min-h-[32px]">
                      {w.description || "No description provided."}
                    </p>
                  </div>

                  <div className="mt-4 pt-3 border-t border-border/60 flex items-center justify-between text-xs">
                    <div className="flex items-center gap-2">
                      <Badge variant="secondary" className="px-2 py-0 text-[10px] font-mono">
                        {nodeCount} {nodeCount === 1 ? "step" : "steps"}
                      </Badge>
                      {w.webhookSlug && (
                        <Badge variant="outline" className="px-1.5 py-0 text-[10px] font-mono flex items-center gap-1">
                          <Globe className="size-2.5" />
                          <span>Webhook</span>
                        </Badge>
                      )}
                      {w.exposeAsTool && (
                        <Badge variant="outline" className="px-1.5 py-0 text-[10px] font-mono flex items-center gap-1 border-primary/40 text-primary">
                          <Sparkles className="size-2.5" />
                          <span>Agent Tool</span>
                        </Badge>
                      )}
                    </div>

                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onClick={(e) => {
                        e.stopPropagation();
                        setPendingDelete(w);
                      }}
                      className="text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100 transition-opacity"
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                </Card>
              );
            })}
          </div>
        )}
      </div>

      {/* Create Workflow Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-base tracking-tight">Create Workflow</DialogTitle>
            <DialogDescription className="text-xs">
              Start from scratch or select a starter template for common automation use cases.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-2 text-xs">
            <div className="space-y-1.5">
              <Label htmlFor="create-wf-name" className="text-xs">Workflow Name</Label>
              <Input
                id="create-wf-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. GitHub Ticket Sync & AI Reply"
                className="h-8 text-xs"
                autoFocus
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="create-wf-desc" className="text-xs">Description (optional)</Label>
              <Input
                id="create-wf-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="What this workflow handles..."
                className="h-8 text-xs"
              />
            </div>

            {/* Template Chooser */}
            <div className="space-y-2 pt-1">
              <Label className="text-xs">Starter Template</Label>
              <div className="grid grid-cols-1 gap-2">
                {WORKFLOW_TEMPLATES.map((t) => {
                  const Icon = t.icon;
                  const isSelected = selectedTemplate === t.id;
                  return (
                    <button
                      key={t.id}
                      type="button"
                      onClick={() => {
                        setSelectedTemplate(t.id);
                        if (!name) setName(t.title);
                      }}
                      className={`flex items-start gap-3 rounded-lg border p-2.5 text-left transition-all ${
                        isSelected
                          ? "border-primary bg-primary/5 ring-1 ring-primary/20"
                          : "border-border hover:bg-muted/40"
                      }`}
                    >
                      <div className="flex size-7 items-center justify-center rounded-md bg-muted border border-border text-primary shrink-0">
                        <Icon className="size-3.5" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="text-xs font-semibold text-foreground">{t.title}</p>
                        <p className="text-[11px] text-muted-foreground line-clamp-1">{t.description}</p>
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button size="sm" onClick={handleCreate} disabled={createWorkflow.isPending || !name.trim()}>
              {createWorkflow.isPending ? "Creating..." : "Create & Open Canvas"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={pendingDelete !== null} onOpenChange={(open) => { if (!open) setPendingDelete(null); }}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Delete Workflow?</DialogTitle>
            <DialogDescription className="text-xs">
              "{pendingDelete?.name}" and all of its past execution runs will be permanently deleted.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setPendingDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={confirmDelete}
              disabled={deleteWorkflow.isPending}
            >
              {deleteWorkflow.isPending ? "Deleting..." : "Delete Workflow"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
