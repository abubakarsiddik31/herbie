import { useCallback, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ApiError, BASE, tryRefresh } from "@/lib/api";
import { useAuth } from "@/stores/auth";
import type { DocumentRec } from "@/lib/types";

export function useDocuments() {
  return useQuery({
    queryKey: ["documents"],
    queryFn: async () => {
      let res = await documentsFetch("GET");
      if (res.status === 401 && (await tryRefresh())) {
        res = await documentsFetch("GET");
      }
      if (!res.ok) {
        // Error bodies mirror apiFetch: { error: { code, message } }.
        let code = "error";
        let message = `failed to load documents (${res.status})`;
        try {
          const body = (await res.json()) as { error?: { code?: string; message?: string } };
          if (body.error?.message) {
            if (body.error.code) code = body.error.code;
            message = body.error.message;
          }
        } catch { /* non-JSON error body */ }
        throw new ApiError(res.status, code, message);
      }
      const body = (await res.json()) as { documents: DocumentRec[] };
      return body.documents;
    },
    // Ingestion runs async after upload: keep refetching while anything is
    // still processing so badges settle to ready/failed on their own.
    refetchInterval: (query) => pollIntervalFor(query.state.data),
  });
}

// How often the documents list refetches: every 2s while ingestion is still
// running somewhere, otherwise not at all.
export function pollIntervalFor(docs: DocumentRec[] | undefined): number | false {
  return docs?.some((d) => d.status === "processing") ? 2000 : false;
}

async function documentsFetch(method: string, path = "", body?: FormData): Promise<Response> {
  const headers = new Headers();
  const token = useAuth.getState().accessToken;
  if (token) headers.set("Authorization", `Bearer ${token}`);
  return fetch(`${BASE}/api/documents${path}`, { method, headers, body, credentials: "include" });
}

/**
 * Upload with progress. XHR rather than fetch because progress events need
 * upload listeners; auth and error shapes mirror apiFetch. A 401 retries
 * once through the refresh endpoint, like apiFetch.
 */
export function uploadWithProgress(
  file: File,
  onProgress: (pct: number) => void,
): Promise<DocumentRec> {
  return new Promise((resolve, reject) => {
    const send = () => {
      const xhr = new XMLHttpRequest();
      xhr.open("POST", `${BASE}/api/documents`);
      const token = useAuth.getState().accessToken;
      if (token) xhr.setRequestHeader("Authorization", `Bearer ${token}`);
      xhr.withCredentials = true;
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100));
      };
      xhr.onload = () => {
        if (xhr.status === 401) {
          void tryRefresh().then((ok) => {
            if (ok) {
              send();
            } else {
              useAuth.getState().clear();
              reject(new ApiError(401, "unauthorized", "session expired"));
            }
          });
          return;
        }
        if (xhr.status < 200 || xhr.status >= 300) {
          let code = "error";
          let message = `upload failed (${xhr.status})`;
          try {
            const body = JSON.parse(xhr.responseText) as { error?: { code: string; message: string } };
            if (body.error) { code = body.error.code; message = body.error.message; }
          } catch { /* non-JSON error body */ }
          reject(new ApiError(xhr.status, code, message));
          return;
        }
        resolve(JSON.parse(xhr.responseText) as DocumentRec);
      };
      xhr.onerror = () => reject(new ApiError(0, "network", "upload failed"));
      const form = new FormData();
      form.append("file", file);
      xhr.send(form);
    };
    send();
  });
}

const ACCEPTED = ".txt,.md,.pdf,.docx,.xlsx,.pptx,.csv,.tsv,.json,.yaml,.yml,.py,.js,.ts,.tsx,.jsx,.go,.rs,.sh,.sql";

export function useDocumentActions() {
  const queryClient = useQueryClient();
  const [progress, setProgress] = useState<number | null>(null);

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ["documents"] });

  const upload = useCallback(
    async (file: File) => {
      setProgress(0);
      try {
        const doc = await uploadWithProgress(file, setProgress);
        invalidate();
        toast.success(`${file.name} indexed`);
        return doc;
      } catch (err) {
        toast.error(err instanceof ApiError ? err.message : "Upload failed");
        throw err;
      } finally {
        setProgress(null);
      }
    },
    [queryClient],
  );

  const remove = useMutation({
    mutationFn: async (id: string) => {
      let res = await documentsFetch("DELETE", `/${id}`);
      if (res.status === 401 && (await tryRefresh())) {
        res = await documentsFetch("DELETE", `/${id}`);
      }
      if (!res.ok && res.status !== 204) {
        throw new ApiError(res.status, "error", "Failed to delete document");
      }
    },
    onSuccess: () => {
      invalidate();
      toast.success("Document deleted");
    },
    onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to delete document"),
  });

  return { upload, remove, progress, accepting: ACCEPTED };
}
