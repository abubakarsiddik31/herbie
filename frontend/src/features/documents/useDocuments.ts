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
      const res = await documentsFetch("GET");
      const body = (await res.json()) as { documents: DocumentRec[] };
      return body.documents;
    },
  });
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

const ACCEPTED = ".txt,.md,.pdf,.docx";

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
