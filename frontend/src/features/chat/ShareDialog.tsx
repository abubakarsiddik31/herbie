import { useEffect, useState } from "react";
import { toast } from "sonner";
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
import { useShareConversation, useShareState, useUnshareConversation } from "./useShare";

interface Props {
  conversation: { id: string; title: string } | null;
  onOpenChange: (open: boolean) => void;
}

export function ShareDialog({ conversation, onOpenChange }: Props) {
  const [freshToken, setFreshToken] = useState<string | null>(null);
  const state = useShareState(conversation?.id ?? null);
  const share = useShareConversation();
  const unshare = useUnshareConversation();

  useEffect(() => {
    if (conversation === null) setFreshToken(null);
  }, [conversation]);

  const link = freshToken === null ? null : `${window.location.origin}/s/${freshToken}`;

  async function copyLink() {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link);
      toast.success("Link copied");
    } catch {
      toast.error("Could not copy the link");
    }
  }

  function createLink() {
    if (!conversation) return;
    share.mutate(conversation.id, {
      onSuccess: (res) => {
        if (res.token) setFreshToken(res.token);
        else void state.refetch();
      },
      onError: () => toast.error("Could not share this conversation"),
    });
  }

  function revoke() {
    if (!conversation) return;
    unshare.mutate(conversation.id, {
      onSuccess: () => {
        setFreshToken(null);
        toast.success("Link revoked");
      },
      onError: () => toast.error("Could not revoke the link"),
    });
  }

  const shared = freshToken !== null || state.data?.shared === true;
  const busy = share.isPending || unshare.isPending || state.isLoading;

  return (
    <Dialog open={conversation !== null} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Share{conversation ? ` “${conversation.title || "Untitled"}”` : ""}</DialogTitle>
          <DialogDescription>Anyone with the link can read this conversation.</DialogDescription>
        </DialogHeader>
        {link !== null ? (
          <div className="flex items-center gap-2">
            <Input readOnly value={link} aria-label="Share link" onFocus={(e) => e.target.select()} />
            <Button variant="outline" onClick={() => void copyLink()}>
              Copy
            </Button>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            {state.data?.shared === true
              ? "This conversation already has an active link. Revoke it and share again to rotate the link."
              : "Create a public read-only link to this conversation."}
          </p>
        )}
        <DialogFooter>
          {shared && (
            <Button variant="destructive" disabled={busy} onClick={revoke}>
              {unshare.isPending ? "Revoking…" : "Revoke link"}
            </Button>
          )}
          {!shared && (
            <Button disabled={busy} onClick={createLink}>
              {share.isPending ? "Sharing…" : "Create link"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
