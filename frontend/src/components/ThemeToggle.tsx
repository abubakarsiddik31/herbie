import { Monitor, Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { Button } from "@/components/ui/button";

const order = ["light", "dark", "system"] as const;
type Mode = (typeof order)[number];

const icons = { light: Sun, dark: Moon, system: Monitor } as const;

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const current = (order as readonly string[]).includes(theme ?? "") ? (theme as Mode) : "system";
  const Icon = icons[current];
  const next = order[(order.indexOf(current) + 1) % order.length];

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={`Theme: ${current}. Switch to ${next} mode.`}
      title={`Theme: ${current}`}
      onClick={() => setTheme(next)}
    >
      <Icon />
    </Button>
  );
}
