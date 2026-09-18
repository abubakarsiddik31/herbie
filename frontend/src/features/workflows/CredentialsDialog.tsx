import { useState } from "react";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  useCreateWorkflowCredential,
  useDeleteWorkflowCredential,
  useWorkflowCredentials,
} from "./useWorkflows";

interface CredentialsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CredentialsDialog({ open, onOpenChange }: CredentialsDialogProps) {
  const { data: credentials, isLoading } = useWorkflowCredentials();
  const createCred = useCreateWorkflowCredential();
  const deleteCred = useDeleteWorkflowCredential();

  const [isAdding, setIsAdding] = useState(false);
  const [name, setName] = useState("");
  const [type, setType] = useState<string>("bearer_token");
  const [secretVal, setSecretVal] = useState("");

  function handleAdd() {
    if (!name.trim() || !secretVal.trim()) {
      toast.error("Name and secret value are required");
      return;
    }
    const cleanName = name.trim().toLowerCase().replace(/\s+/g, "_");
    const dataMap: Record<string, string> = {};
    if (type === "bearer_token") {
      dataMap.token = secretVal.trim();
    } else if (type === "api_key") {
      dataMap.key = secretVal.trim();
    } else {
      dataMap.value = secretVal.trim();
    }

    createCred.mutate(
      { name: cleanName, type, data: dataMap },
      {
        onSuccess: () => {
          toast.success(`Credential "${cleanName}" saved`);
          setName("");
          setSecretVal("");
          setIsAdding(false);
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : "Failed to save credential");
        },
      }
    );
  }

  function handleDelete(id: string, credName: string) {
    deleteCred.mutate(id, {
      onSuccess: () => toast.success(`Deleted credential "${credName}"`),
      onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to delete credential"),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-base">
            <KeyRound className="size-4 text-primary" />
            <span>Workflow Credentials</span>
          </DialogTitle>
          <DialogDescription className="text-xs">
            Manage reusable secrets and API keys for external integrations. Reference them in nodes using <code className="font-mono bg-muted px-1 rounded">{"{{ $credentials.name.token }}"}</code>.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 pt-2">
          {/* List existing credentials */}
          <ScrollArea className="max-h-56 pr-2">
            {isLoading && <p className="text-xs text-muted-foreground">Loading credentials...</p>}
            {!isLoading && (!credentials || credentials.length === 0) && !isAdding && (
              <p className="py-4 text-center text-xs text-muted-foreground">
                No credentials stored yet. Add one below to safely authenticate external APIs.
              </p>
            )}
            <div className="space-y-2">
              {credentials?.map((c) => (
                <div
                  key={c.id}
                  className="flex items-center justify-between rounded-lg border border-border bg-card p-2.5 text-xs shadow-2xs"
                >
                  <div>
                    <p className="font-semibold text-foreground font-mono">{c.name}</p>
                    <p className="text-[10px] text-muted-foreground uppercase">{c.type}</p>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => handleDelete(c.id, c.name)}
                    className="text-muted-foreground hover:text-destructive"
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              ))}
            </div>
          </ScrollArea>

          {/* Add Credential toggle/form */}
          {!isAdding ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setIsAdding(true)}
              className="w-full text-xs gap-1.5"
            >
              <Plus className="size-3.5" />
              <span>Add New Credential</span>
            </Button>
          ) : (
            <div className="rounded-xl border border-border/80 bg-muted/30 p-3.5 space-y-3 text-xs">
              <div className="space-y-1">
                <Label htmlFor="cred-name" className="text-xs">Credential Name</Label>
                <Input
                  id="cred-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. github_token, slack_webhook"
                  className="h-8 font-mono text-xs"
                />
              </div>

              <div className="space-y-1">
                <Label className="text-xs">Credential Type</Label>
                <select
                  value={type}
                  onChange={(e) => setType(e.target.value)}
                  className="h-8 w-full rounded-md border border-input bg-background px-2 text-xs outline-none focus-visible:border-ring"
                >
                  <option value="bearer_token">Bearer Token (OAuth / Personal Access)</option>
                  <option value="api_key">API Key</option>
                  <option value="custom_secret">Custom Secret</option>
                </select>
              </div>

              <div className="space-y-1">
                <Label htmlFor="cred-val" className="text-xs">Secret Value</Label>
                <Input
                  id="cred-val"
                  type="password"
                  value={secretVal}
                  onChange={(e) => setSecretVal(e.target.value)}
                  placeholder="Paste secret token here..."
                  className="h-8 font-mono text-xs"
                />
              </div>

              <div className="flex justify-end gap-2 pt-1">
                <Button variant="ghost" size="sm" onClick={() => setIsAdding(false)}>
                  Cancel
                </Button>
                <Button size="sm" onClick={handleAdd} disabled={createCred.isPending}>
                  {createCred.isPending ? "Saving..." : "Save Credential"}
                </Button>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
