import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";

export interface ProviderKeyStatus {
  provider: "gemini" | "openai" | "anthropic";
  configured: boolean;
}
export interface ProviderKeysResponse {
  allowUserKeys: boolean;
  providers: ProviderKeyStatus[];
}

export function useProviderKeys() {
  return useQuery({
    queryKey: ["provider-keys"],
    queryFn: () => apiFetch<ProviderKeysResponse>("/api/provider-keys"),
  });
}

export function useSetProviderKey() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ provider, apiKey }: { provider: string; apiKey: string }) =>
      apiFetch<void>(`/api/provider-keys/${provider}`, { method: "PUT", json: { apiKey } }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["provider-keys"] }),
  });
}

export function useDeleteProviderKey() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (provider: string) =>
      apiFetch<void>(`/api/provider-keys/${provider}`, { method: "DELETE" }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["provider-keys"] }),
  });
}

export function useTestProviderKey() {
  return useMutation({
    mutationFn: (provider: string) =>
      apiFetch<{ ok: boolean }>(`/api/provider-keys/${provider}/test`, { method: "POST" }),
  });
}
