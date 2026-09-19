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
      <rect x="15.5" y="4.125" width="1" height="4.25" rx="0.5" fill="currentColor" />
      <circle cx="16" cy="3.625" r="1.0625" fill="currentColor" />
      {/* Ear nodes */}
      <rect x="3.5" y="13.625" width="2.5" height="5" rx="1.25" fill="currentColor" />
      <rect x="26" y="13.625" width="2.5" height="5" rx="1.25" fill="currentColor" />
      {/* Head: speech bubble with tail; eyes and smile are cutouts */}
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M 10.75 7.875 H 21.25 A 5.25 5.25 0 0 1 26.5 13.125 V 19.125 A 5.25 5.25 0 0 1 21.25 24.375 H 15 C 14.625 26.5 13.875 28.375 11.125 29.375 C 11.875 28.25 11.625 26.375 11.75 24.375 H 10.75 A 5.25 5.25 0 0 1 5.5 19.125 V 13.125 A 5.25 5.25 0 0 1 10.75 7.875 Z M 10.5 15.375 A 1.75 1.75 0 0 1 14 15.375 L 14 17.125 A 1.75 1.75 0 0 1 10.5 17.125 Z M 18 15.375 A 1.75 1.75 0 0 1 21.5 15.375 L 21.5 17.125 A 1.75 1.75 0 0 1 18 17.125 Z M 13.625 20.625 Q 16 23.125 18.375 20.625 Q 16 21.5 13.625 20.625 Z"
        fill="currentColor"
      />
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
