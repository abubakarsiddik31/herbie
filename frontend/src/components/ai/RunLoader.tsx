// Adapted from beautifului.dev (MIT) by TurboProduct
import { useEffect, useState } from "react";

export function RunLoader() {
  const [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    const t = setInterval(() => setElapsed((s) => s + 1), 1000);
    return () => clearInterval(t);
  }, []);
  return (
    <div className="flex items-center gap-2 text-muted-foreground text-sm">
      <span className="flex gap-1">
        {[0, 1, 2].map((i) => (
          <span key={i} className="size-1.5 rounded-full bg-current animate-bounce"
            style={{ animationDelay: `${i * 150}ms` }} />
        ))}
      </span>
      thinking{elapsed > 2 ? ` · ${elapsed}s` : ""}
    </div>
  );
}
