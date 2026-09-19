import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  addEdge,
  type Connection,
  type Edge,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  ArrowLeft,
  Check,
  History,
  KeyRound,
  Loader2,
  Play,
  Plus,
  Save,
  Settings2,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { apiFetch } from "@/lib/api";
import type { NodeExecutionResult, WorkflowRun } from "@/lib/types";
import { WorkflowNodeComponent } from "@/features/workflows/WorkflowNodeComponent";
import { NodeInspector } from "@/features/workflows/NodeInspector";
import { NodePaletteDialog } from "@/features/workflows/NodePaletteDialog";
import { CredentialsDialog } from "@/features/workflows/CredentialsDialog";
import { WorkflowRunsDialog } from "@/features/workflows/WorkflowRunsDialog";
import { WorkflowSettingsDialog } from "@/features/workflows/WorkflowSettingsDialog";
import { getNodeDefinition, type NodeDefinition } from "@/features/workflows/nodeTypes";
import { useWorkflow, useUpdateWorkflow } from "@/features/workflows/useWorkflows";

const nodeTypes = {
  custom: WorkflowNodeComponent,
};

export function WorkflowCanvasPage() {
  const { workflowId } = useParams<{ workflowId: string }>();
  const navigate = useNavigate();

  const { data: workflow, isLoading, error } = useWorkflow(workflowId);
  const updateWorkflow = useUpdateWorkflow();

  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  // Dialog states
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [credentialsOpen, setCredentialsOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  // Execution state
  const [isRunning, setIsRunning] = useState(false);
  const [nodeResults, setNodeResults] = useState<Record<string, NodeExecutionResult>>({});
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false);

  // Initialize canvas with loaded workflow data
  useEffect(() => {
    if (!workflow) return;
    const initialNodes: Node[] = (workflow.nodes || []).map((n) => {
      const def = getNodeDefinition(n.type);
      return {
        id: n.id,
        type: "custom",
        position: n.position || { x: 100, y: 100 },
        data: {
          name: n.name || def.label,
          type: n.type,
          config: n.data || {},
          executionResult: nodeResults[n.id],
        },
      };
    });

    const initialEdges: Edge[] = (workflow.edges || []).map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
      sourceHandle: e.sourceHandle,
      targetHandle: e.targetHandle,
      animated: true,
      style: { strokeWidth: 2 },
    }));

    setNodes(initialNodes);
    setEdges(initialEdges);
    setHasUnsavedChanges(false);
  }, [workflow]);

  // Sync execution results into node data
  useEffect(() => {
    setNodes((prev) =>
      prev.map((n) => ({
        ...n,
        data: {
          ...n.data,
          executionResult: nodeResults[n.id],
        },
      }))
    );
  }, [nodeResults, setNodes]);

  const onConnect = useCallback(
    (params: Connection) => {
      setEdges((eds) => addEdge({ ...params, animated: true, style: { strokeWidth: 2 } }, eds));
      setHasUnsavedChanges(true);
    },
    [setEdges]
  );

  const selectedNode = useMemo(() => {
    return nodes.find((n) => n.id === selectedNodeId);
  }, [nodes, selectedNodeId]);

  function handleSave() {
    if (!workflow) return;
    const serializableNodes = nodes.map((n) => ({
      id: n.id,
      type: (n.data.type as string) || "manual",
      name: (n.data.name as string) || "",
      position: n.position,
      data: (n.data.config as Record<string, unknown>) || {},
    }));

    const serializableEdges = edges.map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
      sourceHandle: e.sourceHandle || undefined,
      targetHandle: e.targetHandle || undefined,
    }));

    updateWorkflow.mutate(
      {
        id: workflow.id,
        patch: {
          nodes: serializableNodes,
          edges: serializableEdges,
        },
      },
      {
        onSuccess: () => {
          setHasUnsavedChanges(false);
          toast.success("Workflow saved successfully");
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : "Failed to save workflow");
        },
      }
    );
  }

  function handleAddNode(def: NodeDefinition) {
    const newId = `node_${Date.now().toString(36)}`;
    const position = {
      x: nodes.length > 0 ? nodes[nodes.length - 1].position.x + 240 : 150,
      y: nodes.length > 0 ? nodes[nodes.length - 1].position.y : 150,
    };

    const newNode: Node = {
      id: newId,
      type: "custom",
      position,
      data: {
        name: def.label,
        type: def.type,
        config: { ...def.defaultData },
      },
    };

    setNodes((prev) => [...prev, newNode]);
    setSelectedNodeId(newId);
    setHasUnsavedChanges(true);
    toast.success(`Added ${def.label}`);
  }

  function handleUpdateNodeConfig(data: { name: string; config: Record<string, unknown> }) {
    if (!selectedNodeId) return;
    setNodes((prev) =>
      prev.map((n) => {
        if (n.id === selectedNodeId) {
          return {
            ...n,
            data: {
              ...n.data,
              name: data.name,
              config: data.config,
            },
          };
        }
        return n;
      })
    );
    setHasUnsavedChanges(true);
  }

  function handleDeleteNode(id: string) {
    setNodes((prev) => prev.filter((n) => n.id !== id));
    setEdges((prev) => prev.filter((e) => e.source !== id && e.target !== id));
    if (selectedNodeId === id) setSelectedNodeId(null);
    setHasUnsavedChanges(true);
    toast.success("Node deleted");
  }

  async function handleTestWorkflow() {
    if (!workflow) return;
    setIsRunning(true);
    setNodeResults({});

    try {
      // First save if unsaved
      if (hasUnsavedChanges) {
        handleSave();
      }

      const res = await apiFetch<{ run: WorkflowRun }>(`/api/workflows/${workflow.id}/run`, {
        method: "POST",
        json: { input: { test: true, timestamp: new Date().toISOString() } },
      });

      if (res.run) {
        setNodeResults(res.run.nodeResults || {});
        if (res.run.status === "success") {
          toast.success(`Execution finished in ${res.run.durationMs}ms`);
        } else {
          toast.error(`Execution failed: ${res.run.error || "Unknown error"}`);
        }
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Run failed");
    } finally {
      setIsRunning(false);
    }
  }

  function handleToggleActive(checked: boolean) {
    if (!workflow) return;
    updateWorkflow.mutate(
      { id: workflow.id, patch: { isActive: checked } },
      {
        onSuccess: () => {
          toast.success(checked ? "Workflow activated" : "Workflow deactivated");
        },
      }
    );
  }

  if (isLoading) {
    return (
      <div className="flex h-dvh flex-col p-6 space-y-4">
        <Skeleton className="h-10 w-64" />
        <Skeleton className="flex-1 w-full rounded-xl" />
      </div>
    );
  }

  if (error || !workflow) {
    return (
      <div className="flex h-dvh flex-col items-center justify-center gap-4">
        <p className="text-sm text-destructive">Workflow not found or could not be loaded.</p>
        <Button variant="outline" onClick={() => navigate("/workflows")}>
          Back to Workflows
        </Button>
      </div>
    );
  }

  return (
    <div className="flex h-dvh flex-col bg-background text-foreground overflow-hidden">
      {/* Top Navbar */}
      <header className="flex h-13 shrink-0 items-center justify-between border-b border-border bg-card/80 px-4 backdrop-blur-xs z-30">
        <div className="flex items-center gap-3">
          <Link
            to="/workflows"
            className="flex items-center justify-center size-8 rounded-lg border border-border text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            title="Back to workflows"
          >
            <ArrowLeft className="size-4" />
          </Link>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-sm font-bold tracking-tight text-foreground">{workflow.name}</h1>
              <span
                className={`size-2 rounded-full ${
                  workflow.isActive ? "bg-emerald-500 ring-4 ring-emerald-500/20" : "bg-muted-foreground/40"
                }`}
              />
            </div>
            <p className="text-[10px] text-muted-foreground line-clamp-1">
              {workflow.description || "Visual automation flow"}
            </p>
          </div>
        </div>

        {/* Actions Toolbar */}
        <div className="flex items-center gap-2">
          {/* Active Switch */}
          <div className="flex items-center gap-1.5 mr-2">
            <span className="text-xs text-muted-foreground font-medium hidden sm:inline">Active</span>
            <Switch
              checked={workflow.isActive}
              onCheckedChange={handleToggleActive}
              aria-label="Toggle active status"
            />
          </div>

          <Button
            variant="outline"
            size="sm"
            onClick={() => setPaletteOpen(true)}
            className="h-8 gap-1.5 text-xs font-medium shadow-2xs"
          >
            <Plus className="size-3.5" />
            <span className="hidden md:inline">Add Step</span>
          </Button>

          <Button
            variant="outline"
            size="sm"
            onClick={() => setCredentialsOpen(true)}
            className="h-8 gap-1.5 text-xs text-muted-foreground hover:text-foreground shadow-2xs"
            title="Manage external credentials"
          >
            <KeyRound className="size-3.5" />
            <span className="hidden lg:inline">Credentials</span>
          </Button>

          <Button
            variant="outline"
            size="sm"
            onClick={() => setHistoryOpen(true)}
            className="h-8 gap-1.5 text-xs text-muted-foreground hover:text-foreground shadow-2xs"
            title="Past execution history"
          >
            <History className="size-3.5" />
            <span className="hidden lg:inline">Runs</span>
          </Button>

          <Button
            variant="outline"
            size="sm"
            onClick={() => setSettingsOpen(true)}
            className="h-8 gap-1.5 text-xs text-muted-foreground hover:text-foreground shadow-2xs"
            title="Workflow settings"
          >
            <Settings2 className="size-3.5" />
          </Button>

          <Button
            variant="secondary"
            size="sm"
            onClick={handleTestWorkflow}
            disabled={isRunning}
            className="h-8 gap-1.5 text-xs font-semibold shadow-2xs bg-primary/10 hover:bg-primary/20 text-primary border border-primary/20"
          >
            {isRunning ? (
              <>
                <Loader2 className="size-3.5 animate-spin" />
                <span>Running...</span>
              </>
            ) : (
              <>
                <Play className="size-3.5 fill-current" />
                <span>Test Workflow</span>
              </>
            )}
          </Button>

          <Button
            size="sm"
            variant={hasUnsavedChanges ? "brand" : "outline"}
            onClick={handleSave}
            disabled={updateWorkflow.isPending || !hasUnsavedChanges}
            className="h-8 gap-1.5 text-xs shadow-2xs"
          >
            {updateWorkflow.isPending ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : hasUnsavedChanges ? (
              <Save className="size-3.5" />
            ) : (
              <Check className="size-3.5 text-emerald-400" />
            )}
            <span>{hasUnsavedChanges ? "Save" : "Saved"}</span>
          </Button>
        </div>
      </header>

      {/* Main Flow Canvas with Inspector */}
      <div className="flex flex-1 min-h-0 relative">
        <div className="flex-1 h-full">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={(changes) => {
              onNodesChange(changes);
              setHasUnsavedChanges(true);
            }}
            onEdgesChange={(changes) => {
              onEdgesChange(changes);
              setHasUnsavedChanges(true);
            }}
            onConnect={onConnect}
            nodeTypes={nodeTypes}
            onNodeClick={(_, node) => setSelectedNodeId(node.id)}
            onPaneClick={() => setSelectedNodeId(null)}
            fitView
            fitViewOptions={{ padding: 0.2 }}
            defaultEdgeOptions={{ animated: true }}
            className="bg-muted/10"
          >
            <Background gap={18} size={1} />
            <Controls className="!bg-card !border-border !shadow-md !rounded-lg" />
            <MiniMap
              zoomable
              pannable
              className="!bg-card !border-border !rounded-lg !shadow-md"
              nodeColor={() => "var(--color-primary)"}
            />
          </ReactFlow>
        </div>

        {/* Node Configuration Drawer */}
        {selectedNode && (
          <NodeInspector
            node={{
              id: selectedNode.id,
              type: (selectedNode.data.type as string) || "manual",
              name: (selectedNode.data.name as string) || "",
              data: (selectedNode.data.config as Record<string, unknown>) || {},
            }}
            executionResult={nodeResults[selectedNode.id]}
            webhookSlug={workflow.webhookSlug}
            onUpdate={handleUpdateNodeConfig}
            onDelete={() => handleDeleteNode(selectedNode.id)}
            onClose={() => setSelectedNodeId(null)}
          />
        )}
      </div>

      {/* Dialogs */}
      <NodePaletteDialog
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        onSelectNode={handleAddNode}
      />

      <CredentialsDialog
        open={credentialsOpen}
        onOpenChange={setCredentialsOpen}
      />

      <WorkflowRunsDialog
        workflowId={workflow.id}
        open={historyOpen}
        onOpenChange={setHistoryOpen}
      />

      <WorkflowSettingsDialog
        workflow={workflow}
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        onSave={(patch) => {
          updateWorkflow.mutate(
            { id: workflow.id, patch },
            {
              onSuccess: () => toast.success("Settings updated"),
              onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to update settings"),
            }
          );
        }}
      />
    </div>
  );
}
