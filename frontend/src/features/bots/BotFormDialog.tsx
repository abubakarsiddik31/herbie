import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ApiError } from "@/lib/api";
import type { Bot, ModelsResponse } from "@/lib/types";
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
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useModels } from "@/features/chat/useModels";
import { useCreateBot, useUpdateBot } from "@/features/bots/useBots";

export const BOT_COLORS = ["sage", "sky", "amber", "rose", "violet", "slate"] as const;

const PROVIDER_LABEL: Record<string, string> = {
  gemini: "Google Gemini",
  openai: "OpenAI",
  anthropic: "Anthropic",
};

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bot?: Bot;
}

// BotFormDialog creates a new bot (no `bot`) or edits an existing one. The
// model list comes from the server catalog, so bots work with any provider.
export function BotFormDialog({ open, onOpenChange, bot }: Props) {
  const { data: models } = useModels();
  const createBot = useCreateBot();
  const updateBot = useUpdateBot(bot?.id ?? "");
  const saving = bot ? updateBot.isPending : createBot.isPending;

  const [name, setName] = useState("");
  const [emoji, setEmoji] = useState("🤖");
  const [color, setColor] = useState<string>("sage");
  const [instructions, setInstructions] = useState("");
  const [model, setModel] = useState("");
  const [ragEnabled, setRagEnabled] = useState(true);

  // Re-seed the form each time the dialog opens so cancels never leak edits.
  useEffect(() => {
    if (open) {
      setName(bot?.name ?? "");
      setEmoji(bot?.emoji ?? "🤖");
      setColor(bot?.color ?? "sage");
      setInstructions(bot?.instructions ?? "");
      setModel(bot?.model ?? "");
      setRagEnabled(bot?.ragEnabled ?? true);
    }
  }, [open, bot]);

  const byProvider = new Map<string, ModelsResponse["models"]>();
  for (const m of models?.models ?? []) {
    const list = byProvider.get(m.provider) ?? [];
    list.push(m);
    byProvider.set(m.provider, list);
  }
  const effectiveModel = model || models?.default || "";

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    const body = {
      name: name.trim(),
      emoji: emoji.trim() || "🤖",
      color,
      instructions: instructions.trim(),
      model: effectiveModel,
      temperature: null,
      ragEnabled,
    };
    const opts = {
      onSuccess: () => {
        onOpenChange(false);
        toast.success(bot ? "Bot updated" : "Bot created");
      },
      onError: (err: unknown) =>
        toast.error(err instanceof ApiError ? err.message : bot ? "Failed to update bot" : "Failed to create bot"),
    };
    if (bot) updateBot.mutate(body, opts);
    else createBot.mutate(body, opts);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="sm:max-w-md">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{bot ? "Edit bot" : "New bot"}</DialogTitle>
            <DialogDescription>
              A name, a job, and a model — your bot keeps its memory and keeps working.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-4">
            <div className="grid grid-cols-[1fr_5rem] gap-3">
              <div className="grid gap-1.5">
                <Label htmlFor="bot-name">Name</Label>
                <Input
                  id="bot-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Scout"
                  autoFocus
                  required
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="bot-emoji">Emoji</Label>
                <Input
                  id="bot-emoji"
                  value={emoji}
                  onChange={(e) => setEmoji(e.target.value)}
                  maxLength={4}
                  className="text-center text-lg"
                  aria-describedby="bot-emoji-hint"
                />
                <p id="bot-emoji-hint" className="text-[10px] text-muted-foreground">
                  any emoji
                </p>
              </div>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="bot-color">Color</Label>
              <select
                id="bot-color"
                value={color}
                onChange={(e) => setColor(e.target.value)}
                className="h-9 w-full rounded-md border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
              >
                {BOT_COLORS.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="bot-instructions">Job</Label>
              <textarea
                id="bot-instructions"
                value={instructions}
                onChange={(e) => setInstructions(e.target.value)}
                rows={3}
                placeholder="What does this bot do? e.g. Track prices of a watchlist and summarize changes daily."
                className="max-h-40 min-h-18 w-full resize-y rounded-md border border-input bg-transparent px-2.5 py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 field-sizing-content"
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="bot-model">Model</Label>
              <select
                id="bot-model"
                value={effectiveModel}
                onChange={(e) => setModel(e.target.value)}
                className="h-9 w-full rounded-md border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
              >
                {models?.models.length === 0 && <option value="">No models configured</option>}
                {[...byProvider.entries()].map(([provider, list]) => (
                  <optgroup key={provider} label={PROVIDER_LABEL[provider] ?? provider}>
                    {list.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.label}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
            </div>

            <div className="flex items-center justify-between gap-3">
              <div className="grid gap-0.5">
                <Label htmlFor="bot-rag">Search documents</Label>
                <p className="text-muted-foreground text-xs">
                  Let the bot retrieve from your uploaded documents.
                </p>
              </div>
              <Switch id="bot-rag" checked={ragEnabled} onCheckedChange={setRagEnabled} />
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" variant="brand" disabled={!name.trim() || saving}>
              {saving ? "Saving…" : bot ? "Save changes" : "Create bot"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
