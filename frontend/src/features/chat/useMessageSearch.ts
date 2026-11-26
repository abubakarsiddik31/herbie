import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import { useDebouncedValue } from "@/lib/useDebouncedValue";
import type { MessageSearchResponse } from "@/lib/types";

const MIN_QUERY_CHARS = 2;

// Message search is server-side FTS; debounce typing and only issue a
// request once the query has at least two characters.
export function useMessageSearch(rawQuery: string, enabled: boolean, semantic = false) {
  const query = useDebouncedValue(rawQuery.trim(), 250);
  return useQuery({
    queryKey: ["message-search", query, semantic],
    queryFn: () => {
      const params = new URLSearchParams({ q: query });
      if (semantic) params.set("semantic", "1");
      return apiFetch<MessageSearchResponse>(`/api/search/messages?${params.toString()}`);
    },
    enabled: enabled && query.length >= MIN_QUERY_CHARS,
    retry: false,
    staleTime: 30_000,
  });
}
