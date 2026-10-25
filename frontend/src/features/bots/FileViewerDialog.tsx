import type { BotFileMeta } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
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
  file: BotFileMeta | null;
}

// FileViewerDialog shows a workspace file's metadata. There is no file-content
// GET endpoint — bots write files via the write_file tool — so the dialog
// shows the listing details plus a note instead of a preview.
export function FileViewerDialog({ open, onOpenChange, file }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="truncate font-mono text-sm">{file?.path}</DialogTitle>
          <DialogDescription>Workspace file details.</DialogDescription>
        </DialogHeader>

        <dl className="space-y-2 text-sm">
          <div className="flex items-center justify-between gap-4">
            <dt className="text-xs text-muted-foreground">Size</dt>
            <dd>{file ? formatBytes(file.sizeBytes) : "—"}</dd>
          </div>
          <div className="flex items-center justify-between gap-4">
            <dt className="text-xs text-muted-foreground">Updated</dt>
            <dd>{file ? new Date(file.updatedAt).toLocaleString() : "—"}</dd>
          </div>
        </dl>

        <Badge variant="outline" className="w-fit text-[10px] font-normal text-muted-foreground">
          Bots write these via the write_file tool
        </Badge>
      </DialogContent>
    </Dialog>
  );
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
