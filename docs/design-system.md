# Herbie Atomic Design System

Herbie uses a three-layer atomic design token architecture built with OKLCH color spaces, CSS variables, and Tailwind CSS v4.

---

## 1. Brand Identity: Nordic Sage & Mineral Slate

Herbie rejects the generic "AI electric blue and purple" aesthetic in favor of **Nordic Sage & Mineral Slate** — a calm, desaturated, architectural palette inspired by Nordic design studios (Alvar Aalto, Artek, Bang & Olufsen), weathered granite, and eucalyptus.

### Philosophy
- **Zero Visual Fatigue:** Soft, desaturated mineral greens keep prolonged reading and coding sessions calm and easy on the eyes.
- **Architectural Clarity:** Subtle limestone whites in light mode, granite slate in dark mode, and organic precision accents.
- **Dignified Instrument:** Feels like a high-precision creative and technical tool rather than a flashy consumer chatbot.

---

## 2. Token Architecture (Three Layers)

```text
Layer 1: Primitives (Scale)
  └── Raw OKLCH values (--sage-50 through --sage-950, radius scales)
         ↓
Layer 2: Semantics (Purpose & Theme)
  └── Purpose-bound tokens (--brand, --brand-muted, --background, --border)
         ↓
Layer 3: Tailwind Theme (@theme inline)
  └── Atomic utilities (bg-brand, text-brand, ring-brand, etc.)
```

---

## 3. Nordic Mineral Sage Scale (OKLCH)

| Token | OKLCH Value | Approximate Hex | Usage |
|---|---|---|---|
| `--sage-50` | `oklch(0.975 0.010 155)` | `#F4F7F5` | Light mode brand surfaces, subtle tags |
| `--sage-100` | `oklch(0.935 0.020 155)` | `#E5ECE7` | Soft highlights & hover states |
| `--sage-200` | `oklch(0.865 0.035 155)` | `#CCD8CF` | Sage borders in light mode |
| `--sage-300` | `oklch(0.770 0.055 155)` | `#A8BEAF` | Soft sage text in dark mode |
| `--sage-400` | `oklch(0.680 0.070 155)` | `#739B86` | **Hero brand in Dark Mode** (luminous mineral sage) |
| `--sage-500` | `oklch(0.580 0.075 155)` | `#5B816D` | Mineral eucalyptus |
| `--sage-600` | `oklch(0.480 0.070 155)` | `#446252` | **Hero brand in Light Mode** (high contrast, WCAG AAA) |
| `--sage-700` | `oklch(0.390 0.058 155)` | `#324B3E` | Light mode brand hover |
| `--sage-800` | `oklch(0.290 0.042 155)` | `#22342B` | Slate moss shadow |
| `--sage-900` | `oklch(0.200 0.025 155)` | `#15201A` | Granite slate |
| `--sage-950` | `oklch(0.135 0.012 155)` | `#0C130F` | Deep Nordic slate noir |

---

## 4. Semantic Layer Mappings

Semantic tokens automatically adapt between light and dark themes:

### Light Mode (`:root`) — Limestone Alabaster & Alpine Sage
- `--brand`: `var(--sage-600)` (Deep alpine pine sage, high contrast on alabaster, WCAG AAA)
- `--brand-foreground`: `oklch(0.990 0.005 155)` (Pure white)
- `--brand-hover`: `var(--sage-700)`
- `--brand-muted`: `var(--sage-100)`
- `--brand-muted-foreground`: `var(--sage-700)`
- `--brand-border`: `var(--sage-200)`
- `--brand-ring`: `var(--sage-500)`

### Dark Mode (`.dark`) — Granite Slate Noir & Luminous Mineral Sage
- `--brand`: `var(--sage-400)` (Luminous desaturated mineral sage against slate noir)
- `--brand-foreground`: `oklch(0.120 0.012 155)` (Deep granite text on sage badge)
- `--brand-hover`: `oklch(0.740 0.065 155)`
- `--brand-muted`: `oklch(0.240 0.020 155 / 50%)`
- `--brand-muted-foreground`: `var(--sage-300)`
- `--brand-border`: `oklch(0.420 0.035 155 / 40%)`
- `--brand-ring`: `var(--sage-400)`

---

## 5. Tailwind Utility Classes

Developers can compose components with clean atomic tokens:

```tsx
// Hero Brand Button
<Button variant="brand">
  New Chat
</Button>

// Brand-tinted Badge
<Badge variant="brand">
  Claude 3.7 Sonnet
</Badge>

// Interactive Focus Ring
<input className="focus-visible:ring-2 focus-visible:ring-brand" />

// Subtle Sage Accent Text
<span className="text-brand font-semibold">Herbie AI</span>
```

---

## 6. Scaling Rules

1. **Brand Mark & Avatars**: `<BrandMark className="..." />` automatically resolves to deep alpine sage in light mode and luminous mineral sage on granite in dark mode.
2. **Never hardcode hex colors**: Use semantic utilities (`text-brand`, `bg-brand-muted`, `border-border`) so both themes maintain architectural balance.

---

## 7. Logo & Brand Mark

Herbie's logo is the **bubble-bot** — a robot head whose silhouette is a speech bubble. It encodes the product in one shape: a friendly assistant (antenna, ear nodes, capsule eyes, smile) that speaks (the tail). The wordmark is hand-lettered monoline caps with a signature detail: the "I" wears Herbie's antenna as its dot.

### Construction rules
- Single flat color per variant — no gradients, no outlines. Facial features are true cutouts (`fill-rule="evenodd"`), so they adapt to any surface.
- Geometry lives on a 512-unit grid; the 32-unit favicon and `HerbieIcon` are direct `/16` reductions of the same paths.
- Palette is locked to the sage scale: `sage-600 #446252` on light surfaces, `sage-400 #739B86` on dark, `sage-950 #0C130F` for app-icon backgrounds.

### Assets (`docs/assets/`)
| File | Usage |
|---|---|
| `logo-lockup.svg` / `logo-lockup-dark.svg` | Mark + HERBIE wordmark — README hero, site headers, social |
| `logo-mark.svg` / `logo-mark-dark.svg` | Icon only — docs, avatars, compact UI |
| `herbie-icon.png` | Raster app icon (1024px, slate noir tile) |
| `social-card.png` | GitHub social preview (1280×640; upload in repo Settings → Social preview) |

### Don'ts
- Don't recolor the mark outside the sage scale or add gradients/effects.
- Don't place the light variant on dark surfaces (or vice versa) — ship both from the `<picture>` pattern used in the README.
- Don't redraw the face — eyes and smile are proportional cutouts; scaling must preserve the 512-grid geometry.
