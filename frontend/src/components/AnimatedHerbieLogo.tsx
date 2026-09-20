import { useState, useRef, useCallback } from "react";
import { cn } from "@/lib/utils";

export type HerbieMood = "float" | "jumping" | "thinking" | "waving" | "coding" | "celebrate";

export interface AnimatedHerbieLogoProps {
  /**
   * Primary animation mood / state
   * - float: gentle organic hover with expanding ground shadow
   * - jumping: energetic joyful jump (Harvey thinking loop)
   * - thinking: radar antenna signal rings with pulse
   * - waving: friendly side tilt and greeting
   * - coding: holographic visor scan with matrix particles
   * - celebrate: victory bounce with joyful expression
   */
  mood?: HerbieMood;
  /**
   * Size presets or custom via className
   */
  size?: "sm" | "md" | "lg" | "xl" | "hero";
  /**
   * Whether clicking or hovering triggers interactive responses
   */
  interactive?: boolean;
  /**
   * Optional custom speech bubble text to display above Herbie
   */
  bubbleText?: string | null;
  /**
   * Whether to display the hand-lettered HERBIE wordmark below or beside
   */
  showWordmark?: boolean;
  /**
   * Layout orientation with wordmark ('col' | 'row')
   */
  wordmarkOrientation?: "col" | "row";
  /**
   * Additional custom CSS classes for the container
   */
  className?: string;
  /**
   * Callback when user clicks Herbie
   */
  onClick?: () => void;
}

const FUN_QUOTES = [
  "Hey! I'm Herbie — inspired by Fantastic Four's iconic robot helper! 🦘",
  "Zero telemetry, zero secret markups. Pure open source! 🛡️",
  "H.E.R.B.I.E. powered by Go goroutines: fast, concurrent, and lightweight! ⚡",
  "Visual workflows or raw terminal code? I do both! 🎨",
  "Bringing your own model keys? You pay the lab directly! 🪙",
  "Grounded RAG: I search your project docs before checking the web! 📚",
  "Not a cold corporate algorithm. Just your loyal lab companion! 🛠️",
  "Jump with me! Let's automate something awesome! 🚀",
];

const SIZE_MAP = {
  sm: "size-12",
  md: "size-20",
  lg: "size-32",
  xl: "size-44",
  hero: "size-56 sm:size-64 md:size-72",
};

export function AnimatedHerbieLogo({
  mood: moodProp = "float",
  size = "md",
  interactive = true,
  bubbleText,
  showWordmark = false,
  wordmarkOrientation = "col",
  className,
  onClick,
}: AnimatedHerbieLogoProps) {
  const [transientMood, setTransientMood] = useState<HerbieMood | null>(null);
  const [quoteIndex, setQuoteIndex] = useState(0);
  const [showSpeech, setShowSpeech] = useState(false);
  const [mouseOffset, setMouseOffset] = useState({ x: 0, y: 0 });
  const containerRef = useRef<HTMLDivElement>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const activeMood = transientMood ?? moodProp;
  const isSpeechVisible = Boolean(bubbleText || showSpeech);

  // Subtle pupil tracking when interactive and in floating mood
  const handleMouseMove = useCallback(
    (e: React.MouseEvent) => {
      if (!interactive || !containerRef.current) return;
      const rect = containerRef.current.getBoundingClientRect();
      const centerX = rect.left + rect.width / 2;
      const centerY = rect.top + rect.height / 2;
      const dx = (e.clientX - centerX) / (rect.width / 2);
      const dy = (e.clientY - centerY) / (rect.height / 2);
      // Clamp to max 4px offset
      setMouseOffset({
        x: Math.max(-4, Math.min(4, dx * 4)),
        y: Math.max(-3, Math.min(3, dy * 3)),
      });
    },
    [interactive]
  );

  const handleMouseLeave = useCallback(() => {
    setMouseOffset({ x: 0, y: 0 });
  }, []);

  const handleClick = () => {
    if (!interactive) return;

    // Cycle quote
    setQuoteIndex((prev) => (prev + 1) % FUN_QUOTES.length);
    setShowSpeech(true);

    // Playful reaction jump if in float or waving
    if (activeMood === "float" || activeMood === "waving") {
      setTransientMood("jumping");
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      timeoutRef.current = setTimeout(() => {
        setTransientMood(null);
      }, 1600);
    }

    if (onClick) onClick();
  };

  const isJumping = activeMood === "jumping" || activeMood === "celebrate";
  const isThinking = activeMood === "thinking";
  const isWaving = activeMood === "waving";
  const isCoding = activeMood === "coding";

  return (
    <div
      ref={containerRef}
      onMouseMove={handleMouseMove}
      onMouseLeave={handleMouseLeave}
      onClick={handleClick}
      role={interactive ? "button" : "img"}
      tabIndex={interactive ? 0 : undefined}
      onKeyDown={(e) => {
        if (interactive && (e.key === "Enter" || e.key === " ")) {
          e.preventDefault();
          handleClick();
        }
      }}
      aria-label="Herbie the bubble-bot mascot"
      className={cn(
        "relative flex select-none flex-col items-center justify-center transition-all outline-none",
        interactive && "cursor-pointer group focus-visible:ring-2 focus-visible:ring-brand/40 rounded-3xl p-2",
        wordmarkOrientation === "row" ? "sm:flex-row sm:gap-5" : "gap-3",
        className
      )}
    >
      {/* Speech Bubble Popover */}
      {isSpeechVisible && (
        <div
          role="status"
          className="absolute -top-14 sm:-top-16 z-20 max-w-[260px] sm:max-w-xs animate-in fade-in zoom-in-95 duration-200"
        >
          <div className="relative rounded-2xl bg-card px-3.5 py-2 text-xs font-medium text-foreground shadow-lg ring-1 ring-border/80 dark:ring-brand-border/40 backdrop-blur-md">
            <p className="leading-snug text-center text-[12px]">
              {bubbleText || FUN_QUOTES[quoteIndex]}
            </p>
            {/* Speech bubble pointy arrow */}
            <div className="absolute left-1/2 -bottom-1.5 -translate-x-1/2 size-3 rotate-45 border-r border-b border-border bg-card dark:border-brand-border/40" />
          </div>
        </div>
      )}

      {/* Main Mascot Stage Container */}
      <div className={cn("relative flex items-center justify-center", SIZE_MAP[size])}>
        {/* Ambient Halo / Sage Glow behind Herbie */}
        <div
          className={cn(
            "pointer-events-none absolute -inset-4 rounded-full blur-2xl transition-opacity duration-700",
            isThinking || isCoding
              ? "bg-brand/25 dark:bg-brand/35 opacity-100"
              : "bg-brand/10 dark:bg-brand/20 opacity-70 group-hover:opacity-100"
          )}
        />

        {/* SVG Robot Mascot */}
        <svg
          viewBox="0 0 512 512"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
          className="size-full overflow-visible drop-shadow-md transition-transform duration-300"
        >
          <defs>
            {/* Visor Glass Gradient */}
            <linearGradient id="herbieVisorGrad" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stopColor="#15201A" />
              <stop offset="100%" stopColor="#0C130F" />
            </linearGradient>

            {/* Specular Glint Gradient */}
            <linearGradient id="herbieGlint" x1="0%" y1="0%" x2="100%" y2="0%">
              <stop offset="0%" stopColor="white" stopOpacity="0.45" />
              <stop offset="100%" stopColor="white" stopOpacity="0.05" />
            </linearGradient>

            {/* Luminous Eye Radial Glow */}
            <radialGradient id="herbieEyeGlow" cx="50%" cy="50%" r="50%">
              <stop offset="0%" stopColor="#A8BEAF" />
              <stop offset="40%" stopColor="#739B86" />
              <stop offset="100%" stopColor="#446252" />
            </radialGradient>

            {/* Visor Scanline Pattern */}
            <pattern id="herbieGridPattern" width="16" height="8" patternUnits="userSpaceOnUse">
              <line x1="0" y1="4" x2="16" y2="4" stroke="#739B86" strokeOpacity="0.12" strokeWidth="1" />
            </pattern>
          </defs>

          {/* Dynamic Ground Shadow */}
          <ellipse
            cx="256"
            cy="478"
            rx="120"
            ry="24"
            className={cn(
              "fill-sage-950/20 dark:fill-black/60 transition-all duration-300",
              isJumping
                ? "animate-harvey-shadow"
                : "animate-herbie-float-shadow"
            )}
          />

          {/* Master Mascot Group with Mode Animations */}
          <g
            className={cn(
              "transition-all duration-200",
              isJumping && "animate-harvey-jump",
              isWaving && "animate-herbie-wave",
              !isJumping && !isWaving && "animate-herbie-float"
            )}
          >
            {/* Antenna Signal Rings (Thinking / High Concurrency radar waves) */}
            {(isThinking || isCoding || activeMood === "float") && (
              <g className="pointer-events-none">
                <circle
                  cx="256"
                  cy="36"
                  r="18"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2.5"
                  className={cn(
                    "text-brand/50 transition-opacity",
                    isThinking ? "animate-herbie-pulse-ring" : "opacity-0"
                  )}
                />
                <circle
                  cx="256"
                  cy="36"
                  r="28"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  className={cn(
                    "text-brand/35 transition-opacity [animation-delay:400ms]",
                    isThinking ? "animate-herbie-pulse-ring" : "opacity-0"
                  )}
                />
              </g>
            )}

            {/* --- ANTENNA STEM & SENSOR --- */}
            <g transform="translate(0, 22)">
              {/* Antenna Mast */}
              <rect
                x="248"
                y="44"
                width="16"
                height="68"
                rx="8"
                className="fill-brand transition-colors duration-300"
              />

              {/* Antenna Top Glowing Orb */}
              <circle
                cx="256"
                cy="36"
                r="17"
                className={cn(
                  "fill-brand transition-all duration-300",
                  (isThinking || isJumping || isCoding) && "brightness-125 animate-pulse"
                )}
              />
              {/* Antenna Core Specular Spark */}
              <circle
                cx="256"
                cy="34"
                r="7"
                className="fill-brand-muted/80 dark:fill-white"
              />

              {/* --- EAR AUDIO SENSORS --- */}
              {/* Left Ear Node */}
              <g className="transition-transform duration-200">
                <rect
                  x="56"
                  y="196"
                  width="40"
                  height="80"
                  rx="20"
                  className="fill-brand transition-colors duration-300"
                />
                {/* Audio Equalizer Activity Bars on Left Ear */}
                <rect x="68" y="218" width="4" height="36" rx="2" className="fill-brand-muted/40 animate-pulse" />
                <rect x="76" y="226" width="4" height="20" rx="2" className="fill-brand-muted/40 animate-pulse [animation-delay:200ms]" />
                <rect x="84" y="222" width="4" height="28" rx="2" className="fill-brand-muted/40 animate-pulse [animation-delay:500ms]" />
              </g>

              {/* Right Ear Node */}
              <g className="transition-transform duration-200">
                <rect
                  x="416"
                  y="196"
                  width="40"
                  height="80"
                  rx="20"
                  className="fill-brand transition-colors duration-300"
                />
                {/* Audio Equalizer Activity Bars on Right Ear */}
                <rect x="424" y="222" width="4" height="28" rx="2" className="fill-brand-muted/40 animate-pulse [animation-delay:300ms]" />
                <rect x="432" y="218" width="4" height="36" rx="2" className="fill-brand-muted/40 animate-pulse [animation-delay:100ms]" />
                <rect x="440" y="226" width="4" height="20" rx="2" className="fill-brand-muted/40 animate-pulse [animation-delay:400ms]" />
              </g>

              {/* --- MAIN ROBOT HEAD SHELL --- */}
              {/* The Canonical Bubble-Bot Silhouette (Head + Conversational Tail) */}
              <path
                d="M 172 104
                   H 340
                   A 84 84 0 0 1 424 188
                   V 284
                   A 84 84 0 0 1 340 368
                   H 240
                   C 234 402 222 432 178 448
                   C 190 430 186 400 188 368
                   H 172
                   A 84 84 0 0 1 88 284
                   V 188
                   A 84 84 0 0 1 172 104
                   Z"
                className="fill-brand transition-colors duration-300"
              />

              {/* Forehead Specular Curve Reflection */}
              <path
                d="M 160 134 C 210 120 302 120 352 134"
                stroke="url(#herbieGlint)"
                strokeWidth="8"
                strokeLinecap="round"
                fill="none"
              />

              {/* --- VISOR SCREEN (Dark Mineral Faceplate) --- */}
              <rect
                x="130"
                y="156"
                width="252"
                height="174"
                rx="48"
                fill="url(#herbieVisorGrad)"
                stroke="#446252"
                strokeWidth="4"
                strokeOpacity="0.4"
              />

              {/* Holographic Scanline Overlay on Visor */}
              <rect
                x="130"
                y="156"
                width="252"
                height="174"
                rx="48"
                fill="url(#herbieGridPattern)"
                className="pointer-events-none"
              />

              {/* Moving Visor Scanline */}
              <rect
                x="134"
                y="160"
                width="244"
                height="12"
                fill="url(#herbieGlint)"
                className="animate-herbie-visor-scan pointer-events-none"
              />

              {/* Visor Corner Glint */}
              <path
                d="M 152 178 Q 170 178 178 196"
                stroke="white"
                strokeWidth="3"
                strokeOpacity="0.28"
                strokeLinecap="round"
                fill="none"
              />

              {/* --- EYES & EXPRESSIONS --- */}
              {activeMood === "celebrate" ? (
                /* Joyful Arched Eyes (^ ^) */
                <g stroke="#739B86" strokeWidth="8" strokeLinecap="round" fill="none">
                  <path d="M 180 238 Q 200 216 220 238" />
                  <path d="M 292 238 Q 312 216 332 238" />
                </g>
              ) : isCoding ? (
                /* Matrix Bracket Eyes ([ ]) */
                <g className="fill-brand font-mono font-bold text-[32px]">
                  <text x="184" y="244" textAnchor="middle" fill="#739B86">{`{`}</text>
                  <text x="328" y="244" textAnchor="middle" fill="#739B86">{`}`}</text>
                </g>
              ) : (
                /* Canonical Luminous Eyes with Blinking and Pupil Tracking */
                <g className="animate-herbie-blink">
                  {/* Left Eye Capsule Base */}
                  <rect
                    x="180"
                    y="210"
                    width="38"
                    height="52"
                    rx="19"
                    className="fill-brand-muted/20 dark:fill-brand/20"
                  />
                  {/* Left Eye Pupil (Tracks Cursor) */}
                  <g transform={`translate(${mouseOffset.x}, ${mouseOffset.y})`}>
                    <circle
                      cx="199"
                      cy="236"
                      r="16"
                      fill="url(#herbieEyeGlow)"
                      className="transition-transform duration-75"
                    />
                    <circle cx="204" cy="231" r="5" fill="white" />
                  </g>

                  {/* Right Eye Capsule Base */}
                  <rect
                    x="294"
                    y="210"
                    width="38"
                    height="52"
                    rx="19"
                    className="fill-brand-muted/20 dark:fill-brand/20"
                  />
                  {/* Right Eye Pupil (Tracks Cursor) */}
                  <g transform={`translate(${mouseOffset.x}, ${mouseOffset.y})`}>
                    <circle
                      cx="313"
                      cy="236"
                      r="16"
                      fill="url(#herbieEyeGlow)"
                      className="transition-transform duration-75"
                    />
                    <circle cx="318" cy="231" r="5" fill="white" />
                  </g>
                </g>
              )}

              {/* Blushing Cheeks */}
              <circle cx="160" cy="274" r="12" className="fill-brand/35 blur-[1px]" />
              <circle cx="352" cy="274" r="12" className="fill-brand/35 blur-[1px]" />

              {/* --- EXPRESSIVE SMILE --- */}
              {activeMood === "celebrate" ? (
                /* Big Open Laugh / Grin */
                <path
                  d="M 226 280 Q 256 318 286 280 Z"
                  className="fill-brand stroke-brand"
                  strokeWidth="3"
                  strokeLinejoin="round"
                />
              ) : (
                /* Warm Friendly Smile */
                <path
                  d="M 230 282 Q 256 308 282 282"
                  stroke="#739B86"
                  strokeWidth="7"
                  strokeLinecap="round"
                  fill="none"
                />
              )}
            </g>
          </g>
        </svg>
      </div>

      {/* Optional Monoline Hand-Lettered Wordmark */}
      {showWordmark && (
        <div className="flex flex-col items-center justify-center pt-1">
          <svg
            viewBox="0 0 573 134"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
            className="h-8 sm:h-9 w-auto overflow-visible stroke-foreground transition-colors"
            strokeWidth="16"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            {/* H E R B (antenna) I E */}
            <path
              d="M 0 34 V 134 M 88 34 V 134 M 0 84 H 88 
                 M 122 34 V 134 M 122 34 H 194 M 122 84 H 184 M 122 134 H 194 
                 M 228 34 V 134 M 228 34 H 264 A 25 25 0 0 1 264 84 H 228 M 276 84 L 310 134 
                 M 344 34 V 134 M 344 34 H 388 A 25 25 0 0 1 388 84 H 344 M 344 84 H 392 A 25 25 0 0 1 392 134 H 344 
                 M 451 34 V 134 
                 M 501 34 V 134 M 501 34 H 573 M 501 84 H 563 M 501 134 H 573"
            />
            {/* Dot of the 'I' is Herbie's Antenna Sensor Bulb */}
            <circle
              cx="451"
              cy="0"
              r="14"
              className={cn(
                "fill-brand stroke-none transition-all duration-300",
                (isThinking || isJumping) && "animate-pulse brightness-125"
              )}
            />
          </svg>
          <span className="text-[11px] font-mono tracking-widest text-muted-foreground uppercase pt-1">
            The Free AI Workspace
          </span>
        </div>
      )}
    </div>
  );
}
