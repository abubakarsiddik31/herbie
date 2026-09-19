import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { ConversationSettings, ModelsResponse } from "@/lib/types";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  models: ModelsResponse | undefined;
  settings: ConversationSettings;
  onApply: (settings: ConversationSettings) => void;
  saving?: boolean;
}

const PROVIDER_LABEL: Record<string, string> = {
  gemini: "Google Gemini",
  openai: "OpenAI",
  anthropic: "Anthropic",
};

// ConversationSettingsDialog edits the per-conversation model, temperature,
// system prompt, and document search. For a not-yet-created chat the caller
// passes its draft settings and applies them at create time.
export function ConversationSettingsDialog({ open, onOpenChange, models, settings, onApply, saving }: Props) {
  const [model, setModel] = useState(settings.model);
  const [temperature, setTemperature] = useState<number | null>(settings.temperature);
  const [systemPrompt, setSystemPrompt] = useState(settings.systemPrompt);
  const [ragEnabled, setRagEnabled] = useState(settings.ragEnabled);

  // Re-seed the form each time the dialog opens so cancels never leak edits.
  useEffect(() => {
    if (open) {
      setModel(settings.model);
      setTemperature(settings.temperature);
      setSystemPrompt(settings.systemPrompt);
      setRagEnabled(settings.ragEnabled);
    }
  }, [open, settings]);

  const byProvider = new Map<string, ModelsResponse["models"]>();
  for (const m of models?.models ?? []) {
    const list = byProvider.get(m.provider) ?? [];
    list.push(m);
    byProvider.set(m.provider, list);
  }
  const effectiveModel = model || models?.default || "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Conversation settings</DialogTitle>
          <DialogDescription>Model, sampling temperature, and system prompt for this chat.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="model-select">Model</Label>
            <select
              id="model-select"
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

          <div className="grid gap-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="temperature-range">Temperature</Label>
              <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <input
                  type="checkbox"
                  checked={temperature === null}
                  onChange={(e) => setTemperature(e.target.checked ? null : 1)}
                  className="size-3.5 accent-[var(--primary)]"
                />
                Provider default
              </label>
            </div>
            <div className="flex items-center gap-2">
              <input
                id="temperature-range"
                type="range"
                min={0}
                max={2}
                step={0.1}
                value={temperature ?? 1}
                disabled={temperature === null}
                onChange={(e) => setTemperature(Number(e.target.value))}
                className="h-1.5 w-full accent-[var(--primary)] disabled:opacity-40"
              />
              <span className="w-8 text-right text-xs tabular-nums text-muted-foreground">
                {temperature === null ? "—" : temperature.toFixed(1)}
              </span>
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="system-prompt">System prompt</Label>
            <textarea
              id="system-prompt"
              value={systemPrompt}
              onChange={(e) => setSystemPrompt(e.target.value)}
              rows={3}
              placeholder="How should the assistant behave? (empty = default)"
              className="max-h-40 min-h-18 resize-y rounded-md border border-input bg-transparent px-2.5 py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 field-sizing-content"
            />
          </div>

          <div className="flex items-center justify-between gap-3">
            <div className="grid gap-0.5">
              <Label htmlFor="rag-switch">Search documents</Label>
              <p className="text-muted-foreground text-xs">
                Let the assistant retrieve from your uploaded documents in this chat.
              </p>
            </div>
            <Switch id="rag-switch" checked={ragEnabled} onCheckedChange={setRagEnabled} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="brand"
            onClick={() =>
              onApply({
                model: effectiveModel,
                temperature,
                systemPrompt: systemPrompt.trim(),
                ragEnabled,
              })
            }
            disabled={saving}
          >
            {saving ? "Saving…" : "Save"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
