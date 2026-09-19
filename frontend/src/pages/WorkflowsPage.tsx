import { useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import {
  ArrowRight,
  Bot,
  Boxes,
  Eye,
  Globe,
  Plus,
  Search,
  Trash2,
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
import { cn } from "@/lib/utils";
import {
  useCreateWorkflow,
  useDeleteWorkflow,
  useUpdateWorkflow,
  useWorkflows,
} from "@/features/workflows/useWorkflows";
import {
  EXAMPLE_CATEGORIES,
  EXAMPLE_WORKFLOWS,
  type WorkflowExample,
} from "@/features/workflows/exampleWorkflows";
import { WorkflowTemplatePreviewDialog } from "@/features/workflows/WorkflowTemplatePreviewDialog";
import type { Workflow } from "@/lib/types";

export function WorkflowsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { data: workflows, isLoading } = useWorkflows();
  const createWorkflow = useCreateWorkflow();
  const updateWorkflow = useUpdateWorkflow();
  const deleteWorkflow = useDeleteWorkflow();

  useEffect(() => {
    const connected = searchParams.get("connected");
    const error = searchParams.get("error");
    if (connected) {
      toast.success(`${connected.toUpperCase()} account connected successfully via OAuth!`);
      searchParams.delete("connected");
      setSearchParams(searchParams, { replace: true });
    } else if (error) {
      toast.error(`OAuth connection error: ${error}`);
      searchParams.delete("error");
      setSearchParams(searchParams, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  // Navigation tab
  const [activeTab, setActiveTab] = useState<"my" | "templates">("my");

  // Search & Filters
  const [search, setSearch] = useState("");
  const [templateSearch, setTemplateSearch] = useState("");
  const [templateCategory, setTemplateCategory] = useState<string>("All");

  // Dialog states
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>("blank");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [pendingDelete, setPendingDelete] = useState<Workflow | null>(null);
  const [previewTemplate, setPreviewTemplate] = useState<WorkflowExample | null>(null);
  const [cloningId, setCloningId] = useState<string | null>(null);

  // Filtered user workflows
  const filteredWorkflows = useMemo(() => {
    return (workflows || []).filter((w) => {
      const q = search.toLowerCase().trim();
      return !q || w.name.toLowerCase().includes(q) || w.description.toLowerCase().includes(q);
    });
  }, [workflows, search]);

  // Filtered template gallery
  const filteredTemplates = useMemo(() => {
    return EXAMPLE_WORKFLOWS.filter((t) => {
      if (templateCategory !== "All" && t.category !== templateCategory) {
        return false;
      }
      const q = templateSearch.toLowerCase().trim();
      if (!q) return true;
      return (
        t.title.toLowerCase().includes(q) ||
        t.tagline.toLowerCase().includes(q) ||
        t.description.toLowerCase().includes(q) ||
        t.category.toLowerCase().includes(q)
      );
    });
  }, [templateCategory, templateSearch]);

  function handleCreateCustom() {
    if (!name.trim()) {
      toast.error("Please provide a name for the workflow");
      return;
    }

    let initialNodes = [
      { id: "start", type: "manual", name: "Manual Trigger", position: { x: 100, y: 150 }, data: {} },
    ];
    let initialEdges: any[] = [];
    let triggerType = "manual";
    let exposeAsTool = false;
    let toolName = "";
    let toolDescription = "";

    if (selectedTemplateId !== "blank") {
      const tpl = EXAMPLE_WORKFLOWS.find((t) => t.id === selectedTemplateId);
      if (tpl) {
        initialNodes = tpl.nodes as any;
        initialEdges = tpl.edges as any;
        triggerType = tpl.triggerType;
        exposeAsTool = Boolean(tpl.exposeAsTool);
        toolName = tpl.toolName || "";
        toolDescription = tpl.toolDescription || "";
      }
    }

    createWorkflow.mutate(
      {
        name: name.trim(),
        description: description.trim(),
        triggerType,
        nodes: initialNodes,
        edges: initialEdges,
        exposeAsTool,
        toolName,
        toolDescription,
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

  function handleUseTemplate(template: WorkflowExample) {
    setCloningId(template.id);
    createWorkflow.mutate(
      {
        name: template.title,
        description: template.description,
        triggerType: template.triggerType,
        nodes: template.nodes,
        edges: template.edges,
        exposeAsTool: Boolean(template.exposeAsTool),
        toolName: template.toolName || "",
        toolDescription: template.toolDescription || "",
        isActive: true,
      },
      {
        onSuccess: (created) => {
          toast.success(`Created workflow from "${template.title}"`);
          setPreviewTemplate(null);
          navigate(`/workflows/${created.id}`);
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : "Failed to clone template");
        },
        onSettled: () => {
          setCloningId(null);
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
              Visual automation pipelines like n8n with AI nodes, webhooks, and external tool integrations.
            </p>
          </div>
          <Button variant="brand" onClick={() => setCreateOpen(true)} className="gap-2 self-start sm:self-auto shadow-xs">
            <Plus className="size-4" />
            <span>New Workflow</span>
          </Button>
        </div>

        {/* Navigation Tabs */}
        <div className="flex items-center gap-2 border-b border-border text-xs">
          <button
            type="button"
            onClick={() => setActiveTab("my")}
            className={cn(
              "flex items-center gap-2 pb-2.5 px-3 font-semibold transition-colors border-b-2",
              activeTab === "my"
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            )}
          >
            <span>My Workflows</span>
            {workflows && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono h-4">
                {workflows.length}
              </Badge>
            )}
          </button>
          <button
            type="button"
            onClick={() => setActiveTab("templates")}
            className={cn(
              "flex items-center gap-2 pb-2.5 px-3 font-semibold transition-colors border-b-2",
              activeTab === "templates"
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            )}
          >
            <Boxes className="size-3.5 text-primary" />
            <span>Example Templates</span>
            <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono h-4">
              {EXAMPLE_WORKFLOWS.length}
            </Badge>
          </button>
        </div>

        {/* Tab 1: My Workflows */}
        {activeTab === "my" && (
          <div className="space-y-6">
            {/* Search */}
            <div className="relative max-w-md">
              <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search my workflows by name or description..."
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
            ) : filteredWorkflows.length === 0 ? (
              <div className="rounded-2xl border border-dashed border-border p-10 text-center space-y-4">
                <div className="mx-auto flex size-12 items-center justify-center rounded-xl bg-primary/10 text-primary">
                  <WorkflowIcon className="size-6" />
                </div>
                <div>
                  <h3 className="text-sm font-semibold text-foreground">
                    {search ? "No workflows match your search" : "No workflows created yet"}
                  </h3>
                  <p className="text-xs text-muted-foreground mt-1 max-w-sm mx-auto">
                    {search
                      ? "Try searching with a different term."
                      : "Start from scratch or pick an example template below to jump right in."}
                  </p>
                </div>

                {!search && (
                  <div className="pt-2">
                    <p className="text-xs font-semibold text-foreground mb-3">Popular Example Workflows:</p>
                    <div className="grid grid-cols-1 md:grid-cols-3 gap-3 max-w-3xl mx-auto text-left">
                      {EXAMPLE_WORKFLOWS.slice(0, 3).map((tpl) => {
                        const Icon = tpl.icon;
                        return (
                          <div
                            key={tpl.id}
                            className="flex flex-col justify-between rounded-xl border border-border bg-card p-3 shadow-2xs hover:border-primary/40 transition-all"
                          >
                            <div>
                              <div className="flex items-center gap-2 mb-1.5">
                                <Icon className="size-4 text-primary" />
                                <span className="font-semibold text-xs text-foreground line-clamp-1">{tpl.title}</span>
                              </div>
                              <p className="text-[11px] text-muted-foreground line-clamp-2">{tpl.tagline}</p>
                            </div>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => handleUseTemplate(tpl)}
                              disabled={cloningId === tpl.id}
                              className="mt-3 text-xs w-full justify-between"
                            >
                              <span>Use Template</span>
                              <ArrowRight className="size-3" />
                            </Button>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {filteredWorkflows.map((w) => {
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
                                w.isActive ? "bg-emerald-500 ring-2 ring-emerald-500/20" : "bg-muted-foreground/40"
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
                              <Bot className="size-2.5" />
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
        )}

        {/* Tab 2: Example Templates Directory */}
        {activeTab === "templates" && (
          <div className="space-y-6">
            {/* Filter toolbar */}
            <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
              <div className="relative max-w-md flex-1">
                <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={templateSearch}
                  onChange={(e) => setTemplateSearch(e.target.value)}
                  placeholder="Search templates by name, keyword, or integration..."
                  className="pl-9 h-9 text-xs"
                />
              </div>

              {/* Category Pills */}
              <div className="flex items-center gap-1.5 overflow-x-auto pb-1">
                {EXAMPLE_CATEGORIES.map((cat) => (
                  <button
                    key={cat}
                    type="button"
                    onClick={() => setTemplateCategory(cat)}
                    className={cn(
                      "px-2.5 py-1 rounded-lg text-xs font-medium transition-colors whitespace-nowrap",
                      templateCategory === cat
                        ? "bg-primary text-primary-foreground"
                        : "bg-muted text-muted-foreground hover:text-foreground"
                    )}
                  >
                    {cat}
                  </button>
                ))}
              </div>
            </div>

            {/* Template Cards Grid */}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {filteredTemplates.map((tpl) => {
                const Icon = tpl.icon;
                const isCloning = cloningId === tpl.id;
                return (
                  <Card
                    key={tpl.id}
                    className="flex flex-col justify-between p-4 border border-border transition-all hover:border-primary/40 hover:shadow-sm"
                  >
                    <div>
                      <div className="flex items-start justify-between gap-2 mb-2">
                        <div className="flex items-center gap-2.5">
                          <div className="flex size-9 items-center justify-center rounded-xl bg-primary/10 text-primary border border-primary/20 shrink-0">
                            <Icon className="size-4.5" />
                          </div>
                          <div>
                            <h3 className="text-xs font-bold text-foreground line-clamp-1">{tpl.title}</h3>
                            <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-mono mt-0.5">
                              {tpl.category}
                            </Badge>
                          </div>
                        </div>
                        <Badge variant="outline" className="text-[10px] font-mono text-primary border-primary/30 shrink-0">
                          {tpl.badge}
                        </Badge>
                      </div>

                      <p className="text-xs text-foreground font-medium mt-2">{tpl.tagline}</p>
                      <p className="text-[11px] text-muted-foreground line-clamp-2 mt-1 leading-relaxed">
                        {tpl.description}
                      </p>
                    </div>

                    <div className="mt-4 pt-3 border-t border-border/60 flex items-center justify-between gap-2">
                      <span className="text-[11px] text-muted-foreground font-mono">
                        {tpl.nodes.length} nodes
                      </span>

                      <div className="flex items-center gap-1.5">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setPreviewTemplate(tpl)}
                          className="h-7 text-xs gap-1"
                        >
                          <Eye className="size-3" />
                          <span>Preview</span>
                        </Button>
                        <Button
                          size="sm"
                          onClick={() => handleUseTemplate(tpl)}
                          disabled={isCloning}
                          className="h-7 text-xs gap-1 shadow-2xs"
                        >
                          <Plus className="size-3" />
                          <span>{isCloning ? "Cloning..." : "Use"}</span>
                        </Button>
                      </div>
                    </div>
                  </Card>
                );
              })}
            </div>
          </div>
        )}
      </div>

      {/* Preview Dialog */}
      <WorkflowTemplatePreviewDialog
        template={previewTemplate}
        open={previewTemplate !== null}
        onOpenChange={(open) => {
          if (!open) setPreviewTemplate(null);
        }}
        onUseTemplate={handleUseTemplate}
        isCloning={cloningId !== null}
      />

      {/* Create Workflow Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-lg max-h-[85vh] flex flex-col p-0 overflow-hidden">
          <DialogHeader className="p-4 pb-2 border-b border-border">
            <DialogTitle className="text-base tracking-tight">Create Workflow</DialogTitle>
            <DialogDescription className="text-xs">
              Start with an empty canvas or clone an example workflow.
            </DialogDescription>
          </DialogHeader>

          <div className="p-4 space-y-4 text-xs overflow-y-auto">
            <div className="space-y-1.5">
              <Label htmlFor="create-wf-name" className="text-xs">Workflow Name</Label>
              <Input
                id="create-wf-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. My Automation Flow"
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
              <Label className="text-xs">Starting Blueprint</Label>
              <div className="grid grid-cols-1 gap-2 max-h-52 overflow-y-auto pr-1">
                {/* Blank */}
                <button
                  type="button"
                  onClick={() => setSelectedTemplateId("blank")}
                  className={`flex items-start gap-3 rounded-lg border p-2.5 text-left transition-all ${
                    selectedTemplateId === "blank"
                      ? "border-primary bg-primary/5 ring-1 ring-primary/20"
                      : "border-border hover:bg-muted/40"
                  }`}
                >
                  <div className="flex size-7 items-center justify-center rounded-md bg-muted border border-border text-primary shrink-0">
                    <WorkflowIcon className="size-3.5" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-xs font-semibold text-foreground">Blank Workflow</p>
                    <p className="text-[11px] text-muted-foreground">Start fresh with an empty canvas</p>
                  </div>
                </button>

                {/* Example blueprints */}
                {EXAMPLE_WORKFLOWS.map((t) => {
                  const Icon = t.icon;
                  const isSelected = selectedTemplateId === t.id;
                  return (
                    <button
                      key={t.id}
                      type="button"
                      onClick={() => {
                        setSelectedTemplateId(t.id);
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
                        <p className="text-[11px] text-muted-foreground line-clamp-1">{t.tagline}</p>
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>
          </div>

          <DialogFooter className="p-4 border-t border-border">
            <Button variant="outline" size="sm" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button variant="brand" size="sm" onClick={handleCreateCustom} disabled={createWorkflow.isPending || !name.trim()}>
              {createWorkflow.isPending ? "Creating…" : "Create & Open Canvas"}
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
