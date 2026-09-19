import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import type { Conversation, DocumentRec } from "@/lib/types";

export interface Project {
  id: string;
  name: string;
  description: string;
  instructions: string;
  filesCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface ProjectDetail {
  project: Project;
  files: DocumentRec[];
  conversations: Conversation[];
}

export function useProjects() {
  return useQuery({
    queryKey: ["projects"],
    queryFn: async () => {
      const res = await apiFetch<{ projects: Project[] }>("/api/projects");
      return res.projects;
    },
  });
}

export function useProject(id: string | null) {
  return useQuery({
    queryKey: ["project", id],
    queryFn: () => apiFetch<ProjectDetail>(`/api/projects/${id}`),
    enabled: id !== null,
    refetchInterval: (query) => {
      // Poll if any file is processing
      const hasProcessing = query.state.data?.files.some((f) => f.status === "processing");
      return hasProcessing ? 2500 : false;
    },
  });
}

export function useCreateProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; description?: string; instructions?: string }) =>
      apiFetch<Project>("/api/projects", { method: "POST", json: body }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

export function useUpdateProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      ...body
    }: {
      id: string;
      name?: string;
      description?: string;
      instructions?: string;
    }) => apiFetch<Project>(`/api/projects/${id}`, { method: "PATCH", json: body }),
    onSuccess: (_, variables) => {
      void queryClient.invalidateQueries({ queryKey: ["projects"] });
      void queryClient.invalidateQueries({ queryKey: ["project", variables.id] });
    },
  });
}

export function useDeleteProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/api/projects/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

export function useUploadProjectFile(projectId: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (file: File) => {
      if (!projectId) throw new Error("no project selected");
      const form = new FormData();
      form.append("file", file);
      return apiFetch<DocumentRec>(`/api/projects/${projectId}/files`, {
        method: "POST",
        body: form,
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["project", projectId] });
      void queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

export function useCreateProjectConversation(projectId: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body?: { title?: string; model?: string }) => {
      if (!projectId) throw new Error("no project selected");
      return apiFetch<Conversation>(`/api/projects/${projectId}/conversations`, {
        method: "POST",
        json: body ?? {},
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["project", projectId] });
      void queryClient.invalidateQueries({ queryKey: ["conversations"] });
    },
  });
}
