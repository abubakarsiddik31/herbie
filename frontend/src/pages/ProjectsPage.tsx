import React, { useState } from "react";
import { Link, useNavigate } from "react-router";
import { toast } from "sonner";
import {
  ArrowRight,
  FileText,
  FolderGit2,
  FolderPlus,
  Pencil,
  Plus,
  Sidebar,
  Trash2,
} from "lucide-react";
import { ApiError } from "@/lib/api";
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
import { useSidebar } from "@/components/layout/SidebarContext";
import {
  useProjects,
  useCreateProject,
  useUpdateProject,
  useDeleteProject,
  type Project,
} from "@/features/projects/useProjects";

export function ProjectsPage() {
  const navigate = useNavigate();
  const { toggleSidebar, setMobileOpen } = useSidebar();
  const { data: projects, isLoading, isError, refetch } = useProjects();
  const createProject = useCreateProject();
  const updateProject = useUpdateProject();
  const deleteProject = useDeleteProject();

  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [instructions, setInstructions] = useState("");

  const [editingProject, setEditingProject] = useState<Project | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Project | null>(null);

  function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    createProject.mutate(
      {
        name: name.trim(),
        description: description.trim(),
        instructions: instructions.trim(),
      },
      {
        onSuccess: (p) => {
          setCreateOpen(false);
          setName("");
          setDescription("");
          setInstructions("");
          toast.success("Project created");
          navigate(`/projects/${p.id}`);
        },
        onError: (err) =>
          toast.error(err instanceof ApiError ? err.message : "Failed to create project"),
      }
    );
  }

  function handleUpdate(e: React.FormEvent) {
    e.preventDefault();
    if (!editingProject || !editingProject.name.trim()) return;
    updateProject.mutate(
      {
        id: editingProject.id,
        name: editingProject.name.trim(),
        description: editingProject.description,
        instructions: editingProject.instructions,
      },
      {
        onSuccess: () => {
          setEditingProject(null);
          toast.success("Project updated");
        },
        onError: (err) =>
          toast.error(err instanceof ApiError ? err.message : "Failed to update project"),
      }
    );
  }

  function handleDelete() {
    if (!pendingDelete) return;
    deleteProject.mutate(pendingDelete.id, {
      onSuccess: () => {
        setPendingDelete(null);
        toast.success("Project deleted");
      },
      onError: (err) =>
        toast.error(err instanceof ApiError ? err.message : "Failed to delete project"),
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
            <FolderGit2 className="size-4 text-muted-foreground" />
            <h1 className="text-sm font-semibold tracking-tight">Projects</h1>
            {projects && (
              <span className="rounded-full bg-muted px-2 py-0.5 font-mono text-[10px] text-muted-foreground">
                {projects.length}
              </span>
            )}
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Button size="sm" variant="brand" onClick={() => setCreateOpen(true)}>
            <Plus className="size-3.5" /> New Project
          </Button>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-5xl space-y-6 p-6">
          <div className="flex flex-col gap-1">
            <h2 className="text-lg font-semibold tracking-tight">Project Workspaces</h2>
            <p className="text-xs text-muted-foreground">
              Projects combine scoped file management with intelligent chat. When chatting inside a project, Herbie searches your project files first before checking the web.
            </p>
          </div>

          {isLoading && (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-44 rounded-2xl" />
              ))}
            </div>
          )}

          {isError && (
            <div className="flex flex-col items-center gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 p-8 text-center">
              <p className="text-xs text-destructive">Failed to load projects.</p>
              <Button variant="outline" size="sm" onClick={() => void refetch()}>
                Retry
              </Button>
            </div>
          )}

          {!isLoading && !isError && (!projects || projects.length === 0) && (
            <div className="flex flex-col items-center justify-center gap-4 rounded-2xl border border-dashed border-border/80 p-12 text-center">
              <div className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
                <FolderPlus className="size-6" />
              </div>
              <div className="space-y-1">
                <h3 className="text-sm font-semibold">No projects yet</h3>
                <p className="max-w-sm text-xs text-muted-foreground">
                  Create a project to give your conversations a dedicated knowledge base and custom instructions.
                </p>
              </div>
              <Button size="sm" variant="brand" onClick={() => setCreateOpen(true)}>
                <Plus className="size-3.5" /> Create your first project
              </Button>
            </div>
          )}

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {projects?.map((p) => (
              <div
                key={p.id}
                className="group relative flex flex-col justify-between rounded-2xl border border-border/70 bg-card p-5 shadow-xs transition-all hover:border-foreground/25 hover:shadow-sm"
              >
                <div className="space-y-2">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                        <FolderGit2 className="size-4" />
                      </span>
                      <h3 className="font-semibold text-sm truncate">{p.name}</h3>
                    </div>
                    <div className="flex items-center opacity-0 group-hover:opacity-100 transition-opacity">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        aria-label="Edit project"
                        onClick={() => setEditingProject(p)}
                        className="text-muted-foreground hover:text-foreground"
                      >
                        <Pencil className="size-3" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        aria-label="Delete project"
                        onClick={() => setPendingDelete(p)}
                        className="text-muted-foreground hover:text-destructive"
                      >
                        <Trash2 className="size-3" />
                      </Button>
                    </div>
                  </div>

                  <p className="text-xs text-muted-foreground line-clamp-2 min-h-8">
                    {p.description || "No description provided."}
                  </p>
                </div>

                <div className="mt-4 pt-3 border-t border-border/40 flex items-center justify-between text-xs">
                  <span className="flex items-center gap-1.5 text-muted-foreground">
                    <FileText className="size-3.5" />
                    <span>{p.filesCount} {p.filesCount === 1 ? "file" : "files"}</span>
                  </span>

                  <Button size="xs" variant="ghost" asChild className="gap-1 font-medium group-hover:text-primary">
                    <Link to={`/projects/${p.id}`}>
                      <span>Open</span>
                      <ArrowRight className="size-3" />
                    </Link>
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Create Project Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-md">
          <form onSubmit={handleCreate}>
            <DialogHeader>
              <DialogTitle>New Project</DialogTitle>
              <DialogDescription>
                Create a project workspace with dedicated files and custom instructions.
              </DialogDescription>
            </DialogHeader>

            <div className="grid gap-3 py-4">
              <div className="grid gap-1.5">
                <Label htmlFor="proj-name">Project Name</Label>
                <Input
                  id="proj-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Legal Analysis, Backend Refactor"
                  autoFocus
                  required
                />
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="proj-desc">Description (optional)</Label>
                <Input
                  id="proj-desc"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Short description of this project's purpose"
                />
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="proj-inst">Project Instructions (optional)</Label>
                <textarea
                  id="proj-inst"
                  value={instructions}
                  onChange={(e) => setInstructions(e.target.value)}
                  placeholder="Custom guidelines for the AI in this project (e.g. You are a senior Go architect...)"
                  rows={3}
                  className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-xs outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring"
                />
              </div>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreateOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" variant="brand" disabled={!name.trim() || createProject.isPending}>
                {createProject.isPending ? "Creating…" : "Create Project"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Edit Project Dialog */}
      <Dialog open={editingProject !== null} onOpenChange={(open) => !open && setEditingProject(null)}>
        <DialogContent className="sm:max-w-md">
          {editingProject && (
            <form onSubmit={handleUpdate}>
              <DialogHeader>
                <DialogTitle>Edit Project</DialogTitle>
                <DialogDescription>Update project details and instructions.</DialogDescription>
              </DialogHeader>

              <div className="grid gap-3 py-4">
                <div className="grid gap-1.5">
                  <Label htmlFor="edit-name">Project Name</Label>
                  <Input
                    id="edit-name"
                    value={editingProject.name}
                    onChange={(e) =>
                      setEditingProject({ ...editingProject, name: e.target.value })
                    }
                    required
                  />
                </div>

                <div className="grid gap-1.5">
                  <Label htmlFor="edit-desc">Description</Label>
                  <Input
                    id="edit-desc"
                    value={editingProject.description}
                    onChange={(e) =>
                      setEditingProject({ ...editingProject, description: e.target.value })
                    }
                  />
                </div>

                <div className="grid gap-1.5">
                  <Label htmlFor="edit-inst">Project Instructions</Label>
                  <textarea
                    id="edit-inst"
                    value={editingProject.instructions}
                    onChange={(e) =>
                      setEditingProject({ ...editingProject, instructions: e.target.value })
                    }
                    rows={3}
                    className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-xs outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring"
                  />
                </div>
              </div>

              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setEditingProject(null)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={!editingProject.name.trim() || updateProject.isPending}>
                  {updateProject.isPending ? "Saving…" : "Save Changes"}
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={pendingDelete !== null} onOpenChange={(open) => !open && setPendingDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete project?</DialogTitle>
            <DialogDescription>
              “{pendingDelete?.name}” and all of its associated files and project conversations will be permanently deleted.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingDelete(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleteProject.isPending}>
              {deleteProject.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
