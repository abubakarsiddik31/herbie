import { cn } from "@/lib/utils";

export interface HarveyAvatarProps {
  /**
   * When true, Harvey performs an energetic, joyful jumping animation with shadow expansion
   * and glowing antenna pulse, signaling active thinking/processing.
   */
  isProcessing?: boolean;
  /**
   * Optional alias for isProcessing
   */
  animating?: boolean;
  /**
   * Extra classes for sizing or positioning (e.g. `size-7`, `size-8`, `size-12`).
   * Defaults to `size-7.5`.
   */
  className?: string;
}

export function HarveyAvatar({
  isProcessing: isProcessingProp,
  animating,
  className,
}: HarveyAvatarProps) {
  const isProcessing = Boolean(isProcessingProp ?? animating);

  return (
    <div
      className={cn(
        "relative inline-flex shrink-0 items-center justify-center select-none",
        className || "size-7.5"
      )}
      role="img"
      aria-label={isProcessing ? "Harvey is thinking and processing..." : "Harvey avatar"}
    >
      <svg
        viewBox="0 0 36 36"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="size-full overflow-visible"
        aria-hidden="true"
      >
        {/* Floor Shadow: expands/contracts realistically with jump */}
        <ellipse
          cx="18"
          cy="33.5"
          rx="7.5"
          ry="1.8"
          className={cn(
            "fill-emerald-950/20 dark:fill-black/40 transition-opacity duration-300",
            isProcessing ? "animate-harvey-shadow" : "opacity-30"
          )}
        />

        {/* Harvey Mascot Character Group: Jumps playfully during processing */}
        <g className={cn("transition-transform duration-200", isProcessing && "animate-harvey-jump")}>
          {/* Antenna Stem */}
          <line
            x1="18"
            y1="9.5"
            x2="18"
            y2="4.5"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            className="text-emerald-700 dark:text-emerald-400"
          />

          {/* Antenna Glowing Sensor Bulb */}
          <circle
            cx="18"
            cy="3.5"
            r="2.25"
            className={cn(
              "fill-emerald-500 dark:fill-emerald-400 transition-all",
              isProcessing && "animate-pulse"
            )}
          />
          <circle cx="18" cy="3.5" r="1.1" className="fill-emerald-100 dark:fill-white" />

          {/* Ear Nodes / Audio Sensors */}
          <rect
            x="3.2"
            y="13.5"
            width="2.6"
            height="5.5"
            rx="1.3"
            className="fill-emerald-700 dark:fill-emerald-500"
          />
          <rect
            x="30.2"
            y="13.5"
            width="2.6"
            height="5.5"
            rx="1.3"
            className="fill-emerald-700 dark:fill-emerald-500"
          />

          {/* Main Head Shell (No background box! Standalone character) */}
          <rect
            x="5.5"
            y="8.5"
            width="25"
            height="19.5"
            rx="6.5"
            className="fill-emerald-600 dark:fill-emerald-500 transition-colors"
          />

          {/* Glossy specular highlight curve on forehead */}
          <path
            d="M 9.5 11.5 C 13.5 10 22.5 10 26.5 11.5"
            stroke="white"
            strokeOpacity="0.32"
            strokeWidth="1.2"
            strokeLinecap="round"
          />

          {/* Face Visor / Screen */}
          <rect
            x="8.5"
            y="12"
            width="19"
            height="13"
            rx="4.2"
            className="fill-slate-900 dark:fill-slate-950 transition-colors"
          />

          {/* Expressive Luminous Eyes */}
          {/* Left Eye */}
          <circle cx="13.8" cy="17.2" r="2.2" className="fill-emerald-400" />
          <circle cx="14.5" cy="16.5" r="0.75" className="fill-white" />

          {/* Right Eye */}
          <circle cx="22.2" cy="17.2" r="2.2" className="fill-emerald-400" />
          <circle cx="22.9" cy="16.5" r="0.75" className="fill-white" />

          {/* Friendly Smile */}
          <path
            d="M 15.6 21.2 Q 18 22.8 20.4 21.2"
            stroke="#34d399"
            strokeWidth="1.25"
            strokeLinecap="round"
            fill="none"
          />

          {/* Soft Blushing Cheeks */}
          <circle cx="11.2" cy="20" r="1.1" className="fill-emerald-400/35" />
          <circle cx="24.8" cy="20" r="1.1" className="fill-emerald-400/35" />
        </g>
      </svg>
    </div>
  );
}

export const HerbieAvatar = HarveyAvatar;
