import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const SHORTCUTS = [
  { keys: ["Enter"], desc: "Send message" },
  { keys: ["Shift", "Enter"], desc: "Insert new line" },
  { keys: ["⌘/Ctrl", "K"], desc: "Open command palette" },
  { keys: ["⌘/Ctrl", "Shift", "O"], desc: "New chat" },
  { keys: ["⌘/Ctrl", "Shift", "R"], desc: "Regenerate last response" },
  { keys: ["Escape"], desc: "Stop response / close dialog" },
  { keys: ["/"], desc: "Focus composer" },
  { keys: ["?"], desc: "Show keyboard shortcuts" },
];

export function ShortcutsDialog({ open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>Quick actions available across Herbie.</DialogDescription>
        </DialogHeader>
        <div className="divide-y divide-border">
          {SHORTCUTS.map((s) => (
            <div key={s.desc} className="flex items-center justify-between py-2 text-sm">
              <span className="text-foreground">{s.desc}</span>
              <div className="flex items-center gap-1">
                {s.keys.map((k) => (
                  <kbd
                    key={k}
                    className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-xs font-medium text-muted-foreground shadow-xs"
                  >
                    {k}
                  </kbd>
                ))}
              </div>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
