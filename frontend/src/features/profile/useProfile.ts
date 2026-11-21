import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";

export interface Profile {
  defaultInstructions: string;
  defaultTopP: number | null;
  defaultMaxOutputTokens: number;
  defaultReasoning: "low" | "medium" | "high" | "";
}

export function useProfile() {
  return useQuery({
    queryKey: ["profile"],
    queryFn: () => apiFetch<Profile>("/api/profile"),
  });
}

export interface ProfileUpdate {
  defaultInstructions?: string;
  topP?: number | null;
  clearTopP?: boolean;
  maxOutputTokens?: number;
  reasoning?: "low" | "medium" | "high" | "";
}

export function useUpdateProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: string | ProfileUpdate) =>
      apiFetch<void>("/api/profile", {
        method: "PATCH",
        json: typeof input === "string" ? { defaultInstructions: input } : input,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["profile"] });
    },
  });
}
