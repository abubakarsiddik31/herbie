import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ApiError } from "@/lib/api";
import type { BotRoutine } from "@/lib/types";
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
import { useCreateRoutine, useUpdateRoutine } from "@/features/bots/useBots";

export const ROUTINE_TIMEZONES = [
  "UTC",
  "Asia/Dhaka",
  "America/New_York",
  "America/Los_Angeles",
  "Europe/London",
  "Europe/Berlin",
  "Asia/Tokyo",
  "Asia/Singapore",
  "Asia/Kolkata",
  "Australia/Sydney",
] as const;

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  botId: string;
  routine?: BotRoutine;
}

// RoutineFormDialog creates a new routine (no `routine`) or edits an existing
// one. A 400 invalid_schedule from the server (bad cron expression) is shown
// inline under the schedule field in addition to the toast.
export function RoutineFormDialog({ open, onOpenChange, botId, routine }: Props) {
  const createRoutine = useCreateRoutine(botId);
  const updateRoutine = useUpdateRoutine(botId);
  const saving = routine ? updateRoutine.isPending : createRoutine.isPending;

  const [name, setName] = useState("");
  const [schedule, setSchedule] = useState("");
  const [timezone, setTimezone] = useState<string>("UTC");
  const [prompt, setPrompt] = useState("");
  const [scheduleError, setScheduleError] = useState<string | null>(null);

  // Re-seed the form each time the dialog opens so cancels never leak edits
  // and a previous schedule error never carries over.
  useEffect(() => {
    if (open) {
      setName(routine?.name ?? "");
      setSchedule(routine?.schedule ?? "");
      setTimezone(routine?.timezone ?? "UTC");
      setPrompt(routine?.prompt ?? "");
      setScheduleError(null);
    }
  }, [open, routine]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim() || !schedule.trim() || !prompt.trim()) return;
    setScheduleError(null);
    const body = {
      name: name.trim(),
      schedule: schedule.trim(),
      timezone,
      prompt: prompt.trim(),
    };
    try {
      if (routine) {
        await updateRoutine.mutateAsync({ routineId: routine.id, ...body });
        toast.success("Routine updated");
      } else {
        await createRoutine.mutateAsync(body);
        toast.success("Routine created");
      }
      onOpenChange(false);
    } catch (err) {
      if (err instanceof ApiError && err.code === "invalid_schedule") {
        setScheduleError(err.message);
        toast.error(err.message);
      } else {
        toast.error(err instanceof ApiError ? err.message : "Failed to save routine");
      }
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="sm:max-w-md">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{routine ? "Edit routine" : "New routine"}</DialogTitle>
            <DialogDescription>
              The bot runs this prompt on a schedule, with its full memory and tools.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-4">
            <div className="grid gap-1.5">
              <Label htmlFor="routine-name">Name</Label>
              <Input
                id="routine-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Morning digest"
                autoFocus
                required
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="routine-schedule">Schedule</Label>
              <Input
                id="routine-schedule"
                value={schedule}
                onChange={(e) => setSchedule(e.target.value)}
                placeholder="0 8 * * 1-5"
                className="font-mono"
                aria-describedby="routine-schedule-hint"
                required
              />
              <p id="routine-schedule-hint" className="text-[10px] text-muted-foreground">
                m h dom mon dow — e.g. "0 8 * * 1-5" = weekdays 08:00
              </p>
              {scheduleError && (
                <p role="alert" className="text-xs text-destructive">
                  {scheduleError}
                </p>
              )}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="routine-timezone">Timezone</Label>
              <select
                id="routine-timezone"
                value={timezone}
                onChange={(e) => setTimezone(e.target.value)}
                className="h-9 w-full rounded-md border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
              >
                {ROUTINE_TIMEZONES.map((tz) => (
                  <option key={tz} value={tz}>
                    {tz}
                  </option>
                ))}
              </select>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="routine-prompt">Prompt</Label>
              <textarea
                id="routine-prompt"
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                rows={3}
                placeholder="What should the bot do on each run? e.g. Summarize yesterday's emails and post the digest."
                className="max-h-40 min-h-18 w-full resize-y rounded-md border border-input bg-transparent px-2.5 py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 field-sizing-content"
                required
              />
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" variant="brand" disabled={!name.trim() || !schedule.trim() || !prompt.trim() || saving}>
              {saving ? "Saving…" : routine ? "Save changes" : "Create routine"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
