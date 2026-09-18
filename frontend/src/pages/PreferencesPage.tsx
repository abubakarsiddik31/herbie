import { useEffect, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { ArrowLeft } from "lucide-react";
import { ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useProfile, useUpdateProfile } from "@/features/profile/useProfile";

export const MAX_INSTRUCTIONS_CHARS = 4000;

export function PreferencesPage() {
  const { data, isLoading, isError, refetch, isFetching } = useProfile();
  const update = useUpdateProfile();
  const [draft, setDraft] = useState<string | null>(null);

  useEffect(() => {
    if (data && draft === null) setDraft(data.defaultInstructions);
  }, [data, draft]);

  function save() {
    if (draft === null) return;
    update.mutate(draft, {
      onSuccess: () => toast.success("Preferences saved"),
      onError: (err) => toast.error(err instanceof ApiError ? err.message : "Failed to save preferences"),
    });
  }

  const dirty = draft !== null && data !== undefined && draft !== data.defaultInstructions;
  const tooLong = (draft ?? "").length > MAX_INSTRUCTIONS_CHARS;

  return (
    <div className="mx-auto w-full max-w-2xl space-y-6 p-6">
      <div className="flex items-center gap-1">
        <Button variant="ghost" size="icon-sm" asChild aria-label="Back to chat">
          <Link to="/"><ArrowLeft /></Link>
        </Button>
        <h1 className="text-lg font-semibold">Preferences</h1>
      </div>

      {isLoading && <Skeleton className="h-64 w-full" />}

      {isError && (
        <div className="flex items-center gap-3">
          <p className="text-destructive text-sm">Failed to load preferences.</p>
          <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
            Retry
          </Button>
        </div>
      )}

      {data && draft !== null && (
        <Card>
          <CardHeader>
            <CardTitle>Custom instructions</CardTitle>
            <CardDescription>
              Applied to every conversation before its own system prompt, which wins ties.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              rows={8}
              maxLength={MAX_INSTRUCTIONS_CHARS + 100}
              placeholder="e.g. Always answer concisely. Prefer TypeScript examples."
              aria-label="Custom instructions"
              className="min-h-40 w-full rounded-md border border-input bg-transparent p-3 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
            />
            <div className="flex items-center justify-between">
              <p className={tooLong ? "text-destructive text-xs" : "text-muted-foreground text-xs"}>
                {`${draft.length} / ${MAX_INSTRUCTIONS_CHARS}`}
              </p>
              <Button size="sm" disabled={!dirty || tooLong || update.isPending} onClick={save}>
                {update.isPending ? "Saving…" : "Save"}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
