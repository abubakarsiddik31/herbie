import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import type { ModelsResponse } from "@/lib/types";

export function useModels() {
  return useQuery({
    queryKey: ["models"],
    queryFn: () => apiFetch<ModelsResponse>("/api/models"),
    staleTime: Infinity,
  });
}
