import { useMemo, useState } from "react";
import { toast } from "sonner";
import {
  ArrowRight,
  BookOpen,
  Copy,
  Eye,
  Filter,
  Layers,
  Pencil,
  Plus,
  Search,
  ShieldAlert,
  ShieldCheck,
  Sidebar,
  SlidersHorizontal,
  Trash2,
  TriangleAlert,
  Wrench,
  X,
} from "lucide-react";
import { useSidebar } from "@/components/layout/SidebarContext";
import { cn } from "@/lib/utils";
import type { UserTool } from "@/lib/types";
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
  formFromDuplicate,
  formFromTemplate,
  formFromTool,
  formToPayload,
  validateForm,
  type ToolForm,
} from "@/features/tools/toolForm";
import {
  useCreateTool,
  useDeleteTool,
  useTools,
  useUpdateTool,
} from "@/features/tools/useTools";
import { ToolDetailsDialog } from "@/features/tools/ToolDetailsDialog";
import { TemplatePreviewDialog } from "@/features/tools/TemplatePreviewDialog";
import { ToolEditorDialog } from "@/features/tools/ToolEditorDialog";

const METHOD_STYLES: Record<string, string> = {
  GET: "border-emerald-600/30 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400",
  POST: "border-sky-600/30 bg-sky-600/10 text-sky-700 dark:text-sky-400",
  PUT: "border-amber-600/30 bg-amber-600/10 text-amber-700 dark:text-amber-400",
  PATCH: "border-violet-600/30 bg-violet-600/10 text-violet-700 dark:text-violet-400",
  DELETE: "border-rose-600/30 bg-rose-600/10 text-rose-700 dark:text-rose-400",
};

type FilterStatus = "all" | "active" | "paused" | "approval";

// Quick starter template slugs for empty state onboarding
const STARTER_TEMPLATE_SLUGS = ["weather-current", "github-repo", "read-webpage"];

export function ToolsPage() {
  const { toggleSidebar, setMobileOpen } = useSidebar();
  const { data: tools, isLoading, isError, refetch } = useTools();
  const createTool = useCreateTool();
  const updateTool = useUpdateTool();
  const deleteTool = useDeleteTool();

  // Top level view tab
  const [activeView, setActiveView] = useState<"tools" | "templates">("tools");

  // Search & Filter state for user tools
  const [toolsSearch, setToolsSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<FilterStatus>("all");

  // Search & Filter state for templates
  const [templateSearch, setTemplateSearch] = useState("");
  const [templateCategory, setTemplateCategory] = useState<string>("All");

  // Dialog states
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<UserTool | null>(null);
  const [basedOn, setBasedOn] = useState<ToolTemplate | null>(null);
  const [form, setForm] = useState<ToolForm>(emptyForm());

  const [inspectingTool, setInspectingTool] = useState<UserTool | null>(null);
  const [inspectingTemplate, setInspectingTemplate] = useState<ToolTemplate | null>(null);
  const [deleting, setDeleting] = useState<UserTool | null>(null);
  const [addingTemplateSlug, setAddingTemplateSlug] = useState<string | null>(null);

  // Filtered user tools
  const filteredTools = useMemo(() => {
    if (!tools) return [];
    return tools.filter((t) => {
      // Status filter
      if (statusFilter === "active" && !t.enabled) return false;
      if (statusFilter === "paused" && t.enabled) return false;
      if (statusFilter === "approval" && !t.requireApproval) return false;

      // Search filter
      if (toolsSearch.trim() !== "") {
        const query = toolsSearch.toLowerCase().trim();
        const matchesName = t.name.toLowerCase().includes(query);
        const matchesDesc = t.description.toLowerCase().includes(query);
        const matchesMethod = t.method.toLowerCase().includes(query);
        const matchesHost = templateHost(t.urlTemplate).toLowerCase().includes(query);
        return matchesName || matchesDesc || matchesMethod || matchesHost;
      }
      return true;
    });
  }, [tools, statusFilter, toolsSearch]);

  // Filtered templates
  const filteredTemplates = useMemo(() => {
    return TEMPLATES.filter((t) => {
      // Category filter
      if (templateCategory !== "All" && t.category !== templateCategory) {
        return false;
      }
      // Search filter
      if (templateSearch.trim() !== "") {
        const query = templateSearch.toLowerCase().trim();
        const matchesTitle = t.title.toLowerCase().includes(query);
        const matchesTagline = t.tagline.toLowerCase().includes(query);
        const matchesName = t.tool.name.toLowerCase().includes(query);
        const matchesDesc = t.tool.description.toLowerCase().includes(query);
        const matchesHost = templateHost(t.tool.urlTemplate).toLowerCase().includes(query);
        return matchesTitle || matchesTagline || matchesName || matchesDesc || matchesHost;
      }
      return true;
    });
  }, [templateCategory, templateSearch]);

  // Statistics
  const stats = useMemo(() => {
    if (!tools) return { total: 0, active: 0, paused: 0, approval: 0 };
    return {
      total: tools.length,
      active: tools.filter((t) => t.enabled).length,
      paused: tools.filter((t) => !t.enabled).length,
      approval: tools.filter((t) => t.requireApproval).length,
    };
  }, [tools]);

  // Category template count map
  const categoryCounts = useMemo(() => {
    const counts: Record<string, number> = { All: TEMPLATES.length };
    for (const t of TEMPLATES) {
      counts[t.category] = (counts[t.category] || 0) + 1;
    }
    return counts;
  }, []);

  function openEditor(nextForm: ToolForm, template: ToolTemplate | null, editingTool: UserTool | null) {
    setForm(nextForm);
    setBasedOn(template);
    setEditing(editingTool);
    setEditorOpen(true);
  }

  function handleDuplicate(tool: UserTool) {
    openEditor(formFromDuplicate(tool), null, null);
  }

  async function handleSaveTool() {
    const foundErrors = validateForm(form);
    if (Object.keys(foundErrors).length > 0) return;

    const payload = formToPayload(form);
    if (editing) {
      await updateTool.mutateAsync({ id: editing.id, patch: payload });
      toast.success(`Tool “${form.name.trim()}” updated`);
    } else {
      await createTool.mutateAsync(payload);
      toast.success(`Tool “${form.name.trim()}” created`);
    }
    setEditorOpen(false);
  }

  async function handleAddTemplateDirectly(template: ToolTemplate) {
    setAddingTemplateSlug(template.slug);
    try {
      const payload = formToPayload(formFromTemplate(template));
      await createTool.mutateAsync(payload);
      toast.success(`“${template.title}” installed to your tools!`);
      // Switch to tools view so user sees it right away
      setActiveView("tools");
    } catch {
      // Error handled by mutation hook toast
    } finally {
      setAddingTemplateSlug(null);
    }
  }

  function confirmDelete() {
    if (!deleting) return;
    deleteTool.mutate(deleting.id, {
      onSuccess: () => {
        setDeleting(null);
        toast.success("Tool deleted");
      },
    });
  }

  const starterTemplates = useMemo(() => {
    return TEMPLATES.filter((t) => STARTER_TEMPLATE_SLUGS.includes(t.slug));
  }, []);

  return (
    <div className="flex h-full flex-col bg-background">
      {/* App Header Bar */}
      <header className="flex items-center justify-between gap-3 border-b border-border/60 px-4 py-2.5 bg-background/80 backdrop-blur-xs z-10 shrink-0">
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
            <div className="flex size-7 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Wrench className="size-3.5" />
            </div>
            <h1 className="text-sm font-semibold tracking-tight">Agent Tools</h1>
            {tools && tools.length > 0 && (
              <span className="rounded-full bg-muted px-2 py-0.5 font-mono text-[10px] text-muted-foreground font-medium">
                {tools.length} configured
              </span>
            )}
          </div>
        </div>

        <div className="flex items-center gap-2">
          {activeView === "tools" ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setActiveView("templates")}
              className="text-xs gap-1.5"
            >
              <BookOpen className="size-3.5 text-primary" />
              <span>Explore Templates</span>
              <span className="rounded-full bg-muted px-1.5 py-0 text-[10px] font-mono text-muted-foreground">
                18
              </span>
            </Button>
          ) : (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setActiveView("tools")}
              className="text-xs gap-1.5"
            >
              <Layers className="size-3.5" />
              <span>My Tools</span>
              {tools && (
                <span className="rounded-full bg-muted px-1.5 py-0 text-[10px] font-mono text-muted-foreground">
                  {tools.length}
                </span>
              )}
            </Button>
          )}

          <Button
            size="sm"
            onClick={() => openEditor(emptyForm(), null, null)}
            className="text-xs gap-1"
          >
            <Plus className="size-3.5" /> New Tool
          </Button>
        </div>
      </header>

      {/* Main Content Area */}
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-5xl space-y-6 px-4 py-6 sm:px-6">
          {/* Capabilities & Stats Bar */}
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div className="rounded-xl border bg-card p-3.5 shadow-2xs">
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">Active in Agent</span>
                <span className="flex size-2 rounded-full bg-emerald-500 animate-pulse" />
              </div>
              <p className="mt-2 font-mono text-2xl font-bold tracking-tight text-foreground">
                {stats.active}
              </p>
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                Ready for chat invocation
              </p>
            </div>

            <div className="rounded-xl border bg-card p-3.5 shadow-2xs">
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">Require Approval</span>
                <ShieldCheck className="size-3.5 text-amber-600 dark:text-amber-400" />
              </div>
              <p className="mt-2 font-mono text-2xl font-bold tracking-tight text-foreground">
                {stats.approval}
              </p>
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                Prompt for user confirmation
              </p>
            </div>

            <div className="rounded-xl border bg-card p-3.5 shadow-2xs">
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">Total Configured</span>
                <Layers className="size-3.5 text-muted-foreground" />
              </div>
              <p className="mt-2 font-mono text-2xl font-bold tracking-tight text-foreground">
                {stats.total}
              </p>
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                {stats.paused} currently paused
              </p>
            </div>

            <div
              role="button"
              tabIndex={0}
              onClick={() => setActiveView("templates")}
              onKeyDown={(e) => e.key === "Enter" && setActiveView("templates")}
              className="rounded-xl border border-primary/20 bg-primary/5 p-3.5 shadow-2xs cursor-pointer transition-all hover:bg-primary/10 hover:border-primary/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-primary">Prebuilt Library</span>
                <BookOpen className="size-3.5 text-primary" />
              </div>
              <p className="mt-2 font-mono text-2xl font-bold tracking-tight text-foreground">
                {TEMPLATES.length}
              </p>
              <p className="mt-0.5 flex items-center gap-1 text-[11px] text-primary font-medium">
                Browse templates <ArrowRight className="size-3" />
              </p>
            </div>
          </div>

          {/* Primary View Switcher & Toolbar */}
          <div className="flex flex-col gap-4 border-b border-border/60 pb-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              {/* View Toggle Tabs */}
              <div className="flex items-center rounded-xl bg-muted/70 p-1">
                <button
                  type="button"
                  onClick={() => setActiveView("tools")}
                  className={cn(
                    "flex items-center gap-2 rounded-lg px-3.5 py-1.5 text-xs font-medium transition-all",
                    activeView === "tools"
                      ? "bg-background text-foreground shadow-xs"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  <Wrench className="size-3.5" />
                  <span>My Configured Tools</span>
                  {tools && (
                    <Badge variant="secondary" className="px-1.5 py-0 font-mono text-[10px]">
                      {tools.length}
                    </Badge>
                  )}
                </button>
                <button
                  type="button"
                  onClick={() => setActiveView("templates")}
                  className={cn(
                    "flex items-center gap-2 rounded-lg px-3.5 py-1.5 text-xs font-medium transition-all",
                    activeView === "templates"
                      ? "bg-background text-foreground shadow-xs"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  <BookOpen className="size-3.5" />
                  <span>Template Directory</span>
                  <Badge variant="secondary" className="px-1.5 py-0 font-mono text-[10px]">
                    18
                  </Badge>
                </button>
              </div>

              {/* Status explanation */}
              <p className="text-xs text-muted-foreground">
                {activeView === "tools"
                  ? "Tools let Golem query live APIs with structured JSON schema arguments."
                  : "Verified, zero-setup HTTP APIs ready to install in 1 click."}
              </p>
            </div>

            {/* View-specific Search & Filters */}
            {activeView === "tools" ? (
              <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
                {/* Search Input */}
                <div className="relative flex-1 min-w-[220px] max-w-md">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground pointer-events-none" />
                  <Input
                    value={toolsSearch}
                    onChange={(e) => setToolsSearch(e.target.value)}
                    placeholder="Search tools by name, description, method, host..."
                    className="h-9 pl-9 pr-8 text-xs bg-card"
                  />
                  {toolsSearch && (
                    <button
                      type="button"
                      onClick={() => setToolsSearch("")}
                      className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    >
                      <X className="size-3.5" />
                    </button>
                  )}
                </div>

                {/* Status Filter Chips */}
                <div className="flex flex-wrap items-center gap-1.5">
                  {(
                    [
                      { id: "all", label: "All", count: stats.total },
                      { id: "active", label: "Active", count: stats.active },
                      { id: "paused", label: "Paused", count: stats.paused },
                      { id: "approval", label: "Needs Approval", count: stats.approval },
                    ] as const
                  ).map((f) => (
                    <button
                      key={f.id}
                      type="button"
                      onClick={() => setStatusFilter(f.id)}
                      className={cn(
                        "flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-colors",
                        statusFilter === f.id
                          ? "border-primary bg-primary text-primary-foreground"
                          : "border-border text-muted-foreground hover:bg-muted hover:text-foreground",
                      )}
                    >
                      <span>{f.label}</span>
                      <span
                        className={cn(
                          "rounded-full px-1.5 py-0 text-[10px] font-mono",
                          statusFilter === f.id
                            ? "bg-primary-foreground/20 text-primary-foreground"
                            : "bg-muted text-muted-foreground",
                        )}
                      >
                        {f.count}
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            ) : (
              <div className="flex flex-col gap-3 pt-1">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  {/* Template Search */}
                  <div className="relative flex-1 min-w-[240px] max-w-md">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground pointer-events-none" />
                    <Input
                      value={templateSearch}
                      onChange={(e) => setTemplateSearch(e.target.value)}
                      placeholder="Search templates (e.g. crypto, github, weather, news)..."
                      className="h-9 pl-9 pr-8 text-xs bg-card"
                    />
                    {templateSearch && (
                      <button
                        type="button"
                        onClick={() => setTemplateSearch("")}
                        className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                      >
                        <X className="size-3.5" />
                      </button>
                    )}
                  </div>

                  <span className="text-xs text-muted-foreground font-mono">
                    Showing {filteredTemplates.length} of {TEMPLATES.length} templates
                  </span>
                </div>

                {/* Category Pills */}
                <div className="flex flex-wrap gap-1.5">
                  {["All", ...TEMPLATE_CATEGORIES].map((cat) => {
                    const isSelected = templateCategory === cat;
                    const count = categoryCounts[cat] || 0;
                    return (
                      <button
                        key={cat}
                        type="button"
                        onClick={() => setTemplateCategory(cat)}
                        className={cn(
                          "flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-colors",
                          isSelected
                            ? "border-primary bg-primary text-primary-foreground"
                            : "border-border text-muted-foreground hover:bg-muted hover:text-foreground",
                        )}
                      >
                        <span>{cat}</span>
                        <span
                          className={cn(
                            "rounded-full px-1.5 py-0 text-[10px] font-mono",
                            isSelected
                              ? "bg-primary-foreground/20 text-primary-foreground"
                              : "bg-muted text-muted-foreground",
                          )}
                        >
                          {count}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>
            )}
          </div>

          {/* VIEW 1: USER CONFIGURED TOOLS */}
          {activeView === "tools" && (
            <div>
              {isLoading && (
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {[0, 1, 2, 3, 4, 5].map((i) => (
                    <Skeleton key={i} className="h-44 rounded-xl" />
                  ))}
                </div>
              )}

              {isError && (
                <div className="flex flex-col items-center gap-3 rounded-xl border border-destructive/30 bg-destructive/5 px-6 py-12 text-center">
                  <TriangleAlert className="size-5 text-destructive" />
                  <p className="text-sm font-medium text-foreground">Couldn’t load your tools</p>
                  <p className="text-xs text-muted-foreground max-w-sm">
                    An error occurred while communicating with the tools API.
                  </p>
                  <Button variant="outline" size="sm" onClick={() => void refetch()}>
                    Try again
                  </Button>
                </div>
              )}

              {/* Empty state: No tools installed at all */}
              {!isLoading && !isError && tools?.length === 0 && (
                <div className="rounded-2xl border border-dashed bg-card/60 p-8 text-center space-y-6">
                  <div className="mx-auto flex size-14 items-center justify-center rounded-2xl border bg-muted/80 text-foreground">
                    <Wrench className="size-7 text-primary" />
                  </div>
                  <div className="space-y-1.5 max-w-md mx-auto">
                    <h3 className="text-base font-semibold">Equip Golem with Live Web APIs</h3>
                    <p className="text-xs text-muted-foreground leading-relaxed">
                      Custom tools allow Golem to fetch live weather data, check GitHub repositories, search Hacker News, read webpages, and perform actions mid-conversation.
                    </p>
                  </div>

                  {/* Starter recommendations */}
                  <div className="space-y-3 max-w-xl mx-auto text-left">
                    <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider text-center">
                      Quick Install Starters (Zero Configuration)
                    </p>
                    <div className="grid gap-2 sm:grid-cols-3">
                      {starterTemplates.map((t) => {
                        const Icon = t.icon;
                        const isAdding = addingTemplateSlug === t.slug;
                        return (
                          <div
                            key={t.slug}
                            className="flex flex-col justify-between rounded-xl border bg-card p-3 shadow-xs hover:border-foreground/25 transition-all"
                          >
                            <div>
                              <div className="flex items-center gap-2">
                                <span className="flex size-7 items-center justify-center rounded-lg border bg-muted/60">
                                  <Icon className="size-3.5 text-foreground" />
                                </span>
                                <span className="truncate text-xs font-semibold">{t.title}</span>
                              </div>
                              <p className="mt-2 line-clamp-2 text-[11px] text-muted-foreground">
                                {t.tagline}
                              </p>
                            </div>
                            <Button
                              size="xs"
                              variant="outline"
                              className="mt-3 w-full text-xs font-medium"
                              onClick={() => void handleAddTemplateDirectly(t)}
                              disabled={isAdding}
                            >
                              <Plus className="size-3 mr-1" />
                              {isAdding ? "Adding…" : "Add Tool"}
                            </Button>
                          </div>
                        );
                      })}
                    </div>
                  </div>

                  <div className="flex items-center justify-center gap-3 pt-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setActiveView("templates")}
                    >
                      <BookOpen className="size-3.5 mr-1" /> Browse All 18 Templates
                    </Button>
                    <Button
                      size="sm"
                      onClick={() => openEditor(emptyForm(), null, null)}
                    >
                      <Plus className="size-3.5 mr-1" /> Create Custom Tool
                    </Button>
                  </div>
                </div>
              )}

              {/* Empty state: Tools exist, but search / filter returns 0 */}
              {!isLoading && !isError && tools && tools.length > 0 && filteredTools.length === 0 && (
                <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
                  <Filter className="size-8 text-muted-foreground/60 mb-2" />
                  <p className="text-sm font-medium text-foreground">No tools match your criteria</p>
                  <p className="text-xs text-muted-foreground mt-1">
                    Try adjusting your search query or clearing the status filter.
                  </p>
                  <Button
                    variant="outline"
                    size="sm"
                    className="mt-4"
                    onClick={() => {
                      setToolsSearch("");
                      setStatusFilter("all");
                    }}
                  >
                    Reset Filters
                  </Button>
                </div>
              )}

              {/* Tools Grid */}
              {!isLoading && !isError && filteredTools.length > 0 && (
                <div className="grid gap-3.5 sm:grid-cols-2 lg:grid-cols-3">
                  {filteredTools.map((tool) => {
                    const host = templateHost(tool.urlTemplate);
                    const isUpdating = updateTool.isPending;

                    return (
                      <div
                        key={tool.id}
                        className={cn(
                          "group relative flex flex-col justify-between rounded-xl border bg-card p-4 shadow-2xs transition-all duration-200",
                          "hover:-translate-y-0.5 hover:shadow-md hover:border-foreground/25",
                          !tool.enabled && "border-dashed bg-muted/10",
                        )}
                      >
                        <div>
                          {/* Card Header: Method + Name + Action Buttons */}
                          <div className="flex items-start justify-between gap-2">
                            <div className="flex items-center gap-1.5 min-w-0">
                              <span
                                className={cn(
                                  "shrink-0 rounded-md border px-1.5 py-0.5 font-mono text-[10px] font-semibold",
                                  METHOD_STYLES[tool.method] ?? "border-border bg-muted text-muted-foreground",
                                )}
                              >
                                {tool.method}
                              </span>
                              <span
                                className="truncate font-mono text-xs font-semibold text-foreground cursor-pointer hover:underline"
                                onClick={() => setInspectingTool(tool)}
                                title={tool.name}
                              >
                                {tool.name}
                              </span>
                            </div>

                            {/* Actions cluster */}
                            <div className="flex items-center gap-0.5 shrink-0">
                              <Button
                                variant="ghost"
                                size="icon-xs"
                                aria-label={`Inspect ${tool.name}`}
                                title="Inspect tool details"
                                onClick={() => setInspectingTool(tool)}
                                className="text-muted-foreground hover:text-foreground"
                              >
                                <Eye className="size-3" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon-xs"
                                aria-label={`Edit ${tool.name}`}
                                title="Edit tool"
                                onClick={() => openEditor(formFromTool(tool), null, tool)}
                                className="text-muted-foreground hover:text-foreground"
                              >
                                <Pencil className="size-3" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon-xs"
                                aria-label={`Duplicate ${tool.name}`}
                                title="Duplicate tool"
                                onClick={() => handleDuplicate(tool)}
                                className="text-muted-foreground hover:text-foreground"
                              >
                                <Copy className="size-3" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon-xs"
                                aria-label={`Delete ${tool.name}`}
                                title="Delete tool"
                                className="text-muted-foreground hover:text-destructive"
                                onClick={() => setDeleting(tool)}
                              >
                                <Trash2 className="size-3" />
                              </Button>
                            </div>
                          </div>

                          {/* Description */}
                          <p
                            className="mt-2.5 line-clamp-2 min-h-8 text-xs leading-relaxed text-muted-foreground cursor-pointer"
                            onClick={() => setInspectingTool(tool)}
                          >
                            {tool.description}
                          </p>

                          {/* Meta Endpoint & Params */}
                          <div className="mt-3 flex flex-wrap items-center gap-1.5 text-[11px]">
                            <span className="rounded-md bg-muted/60 px-2 py-0.5 font-mono text-[10px] text-muted-foreground truncate max-w-[160px]">
                              {host}
                            </span>
                            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                              {tool.params.length === 0
                                ? "0 params"
                                : `${tool.params.length} param${tool.params.length === 1 ? "" : "s"}`}
                            </span>
                          </div>
                        </div>

                        {/* Card Footer: Status Badges & Toggle */}
                        <div className="mt-4 flex items-center justify-between border-t border-border/60 pt-3">
                          <div className="flex items-center gap-1.5 min-w-0">
                            {tool.requireApproval ? (
                              <Badge
                                variant="outline"
                                className="gap-1 border-amber-500/40 bg-amber-500/10 text-[10px] font-medium text-amber-700 dark:text-amber-400 px-1.5 py-0"
                              >
                                <ShieldCheck className="size-3" /> approval
                              </Badge>
                            ) : (
                              <span className="flex items-center gap-1 text-[10px] text-muted-foreground">
                                <span
                                  className={cn(
                                    "size-1.5 rounded-full",
                                    tool.enabled ? "bg-emerald-500" : "bg-muted-foreground/40",
                                  )}
                                />
                                {tool.enabled ? "Active" : "Paused"}
                              </span>
                            )}
                          </div>

                          <label className="flex items-center gap-2 cursor-pointer text-xs text-muted-foreground select-none">
                            <span className="text-[11px] font-medium">
                              {tool.enabled ? "Enabled" : "Disabled"}
                            </span>
                            <Switch
                              checked={tool.enabled}
                              onCheckedChange={(enabled) =>
                                updateTool.mutate({ id: tool.id, patch: { enabled } })
                              }
                              aria-label={`Toggle ${tool.name} active`}
                              disabled={isUpdating}
                            />
                          </label>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          {/* VIEW 2: TEMPLATE DIRECTORY */}
          {activeView === "templates" && (
            <div>
              {filteredTemplates.length === 0 ? (
                <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
                  <Search className="size-8 text-muted-foreground/60 mb-2" />
                  <p className="text-sm font-medium text-foreground">No templates match “{templateSearch}”</p>
                  <p className="text-xs text-muted-foreground mt-1">
                    Try searching for different keywords or select “All” categories.
                  </p>
                  <Button
                    variant="outline"
                    size="sm"
                    className="mt-4"
                    onClick={() => {
                      setTemplateSearch("");
                      setTemplateCategory("All");
                    }}
                  >
                    Reset Template Search
                  </Button>
                </div>
              ) : (
                <div className="grid gap-3.5 sm:grid-cols-2 lg:grid-cols-3">
                  {filteredTemplates.map((template) => {
                    const Icon = template.icon;
                    const host = templateHost(template.tool.urlTemplate);
                    const isAdding = addingTemplateSlug === template.slug;

                    return (
                      <div
                        key={template.slug}
                        className="group flex flex-col justify-between rounded-xl border bg-card p-4 shadow-2xs transition-all duration-200 hover:-translate-y-0.5 hover:shadow-md hover:border-foreground/25"
                      >
                        <div>
                          {/* Template Header */}
                          <div className="flex items-start justify-between gap-2">
                            <div className="flex items-center gap-2.5">
                              <span className="flex size-9 items-center justify-center rounded-xl border bg-muted/60 text-foreground transition-transform group-hover:scale-105">
                                <Icon className="size-4" />
                              </span>
                              <div>
                                <h3 className="text-xs font-semibold tracking-tight text-foreground">
                                  {template.title}
                                </h3>
                                <div className="flex items-center gap-1.5 mt-0.5">
                                  <Badge
                                    variant="outline"
                                    className="font-normal text-[10px] px-1 py-0 h-4 text-muted-foreground"
                                  >
                                    {template.category}
                                  </Badge>
                                  <span className="rounded bg-emerald-600/10 px-1 py-0 font-mono text-[9px] font-semibold text-emerald-700 dark:text-emerald-400">
                                    {template.tool.method}
                                  </span>
                                </div>
                              </div>
                            </div>
                          </div>

                          {/* Tagline */}
                          <p className="mt-2.5 line-clamp-2 min-h-8 text-xs leading-relaxed text-muted-foreground">
                            {template.tagline}
                          </p>

                          {/* Host & Parameters Meta */}
                          <div className="mt-3 flex flex-wrap items-center gap-1.5 text-[11px]">
                            <span className="rounded-md bg-muted/60 px-2 py-0.5 font-mono text-[10px] text-muted-foreground truncate max-w-[160px]">
                              {host}
                            </span>
                            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                              {template.tool.params.length === 0
                                ? "0 params"
                                : `${template.tool.params.length} param${template.tool.params.length === 1 ? "" : "s"}`}
                            </span>
                          </div>
                        </div>

                        {/* Template Footer Actions */}
                        <div className="mt-4 flex items-center justify-between gap-2 border-t border-border/60 pt-3">
                          <Button
                            variant="ghost"
                            size="xs"
                            onClick={() => setInspectingTemplate(template)}
                            className="text-xs text-muted-foreground hover:text-foreground"
                          >
                            <Eye className="size-3 mr-1" /> Preview
                          </Button>

                          <div className="flex items-center gap-1.5">
                            <Button
                              variant="outline"
                              size="xs"
                              onClick={() => openEditor(formFromTemplate(template), template, null)}
                              title="Customize in editor before saving"
                              className="text-xs"
                            >
                              <SlidersHorizontal className="size-3" />
                            </Button>
                            <Button
                              size="xs"
                              onClick={() => void handleAddTemplateDirectly(template)}
                              disabled={isAdding}
                              className="text-xs font-medium"
                            >
                              <Plus className="size-3 mr-1" />
                              {isAdding ? "Adding…" : "Add"}
                            </Button>
                          </div>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Tool Details Inspector Dialog */}
      <ToolDetailsDialog
        tool={inspectingTool}
        open={inspectingTool !== null}
        onOpenChange={(open) => {
          if (!open) setInspectingTool(null);
        }}
        onEdit={(tool) => openEditor(formFromTool(tool), null, tool)}
        onDelete={(tool) => setDeleting(tool)}
        onToggle={(tool, enabled) => updateTool.mutate({ id: tool.id, patch: { enabled } })}
        onDuplicate={handleDuplicate}
      />

      {/* Template Preview Dialog */}
      <TemplatePreviewDialog
        template={inspectingTemplate}
        open={inspectingTemplate !== null}
        onOpenChange={(open) => {
          if (!open) setInspectingTemplate(null);
        }}
        onUseTemplate={(template) => void handleAddTemplateDirectly(template)}
        onCustomize={(template) => openEditor(formFromTemplate(template), template, null)}
        isAdding={addingTemplateSlug !== null}
      />

      {/* Tool Creation & Editing Dialog */}
      <ToolEditorDialog
        open={editorOpen}
        onOpenChange={(open) => {
          if (!open) setEditorOpen(false);
        }}
        form={form}
        setForm={setForm}
        editing={editing}
        basedOn={basedOn}
        onSave={handleSaveTool}
        saving={createTool.isPending || updateTool.isPending}
      />

      {/* Deletion Confirmation Dialog */}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
      >
        <DialogContent showCloseButton={false} className="sm:max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-2 text-destructive">
              <ShieldAlert className="size-5" />
              <DialogTitle className="text-base">Delete Tool?</DialogTitle>
            </div>
            <DialogDescription className="text-xs text-muted-foreground mt-1.5 leading-relaxed">
              Are you sure you want to delete <code className="font-mono font-semibold text-foreground">“{deleting?.name}”</code>? Golem will no longer be able to call this tool in future conversations. Previous chat messages that referenced it will remain intact.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="mt-4 gap-2">
            <Button variant="outline" size="sm" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={confirmDelete}
              disabled={deleteTool.isPending}
            >
              {deleteTool.isPending ? "Deleting…" : "Delete Tool"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
