import { cn } from "@/lib/utils";

export function HerbieIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={cn("size-4 shrink-0", className)}
      aria-hidden="true"
    >
      {/* Antenna */}
      <rect x="14.75" y="2" width="2.5" height="5" rx="1.25" fill="currentColor" />
      <circle cx="16" cy="2.5" r="2" fill="currentColor" />
      {/* Ear nodes */}
      <rect x="2" y="13" width="2" height="6" rx="1" fill="currentColor" />
      <rect x="28" y="13" width="2" height="6" rx="1" fill="currentColor" />
      {/* Head with visor cutout */}
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M 11 6.5 H 21 A 6.5 6.5 0 0 1 27.5 13 V 21 A 6.5 6.5 0 0 1 21 27.5 H 11 A 6.5 6.5 0 0 1 4.5 21 V 13 A 6.5 6.5 0 0 1 11 6.5 Z M 11.5 11 H 20.5 A 4 4 0 0 1 24.5 15 A 4 4 0 0 1 20.5 19 H 11.5 A 4 4 0 0 1 7.5 15 A 4 4 0 0 1 11.5 11 Z"
        fill="currentColor"
      />
      {/* Twin eyes */}
      <circle cx="12" cy="15" r="1.8" fill="currentColor" />
      <circle cx="20" cy="15" r="1.8" fill="currentColor" />
    </svg>
  );
}

export function BrandMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "flex shrink-0 items-center justify-center rounded-lg bg-brand text-brand-foreground shadow-xs shadow-brand/20 ring-1 ring-brand-border/30",
        className,
      )}
    >
      <HerbieIcon className="size-3.5" />
    </span>
  );
}
