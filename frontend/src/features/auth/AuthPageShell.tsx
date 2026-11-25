import type { ReactNode } from "react";
import { Link } from "react-router";
import { AnimatedHerbieLogo } from "@/components/AnimatedHerbieLogo";

export function AuthPageShell({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
}) {
  return (
    <div className="relative min-h-screen flex flex-col justify-between bg-background text-foreground antialiased">
      <div className="pointer-events-none absolute -top-40 left-1/2 -translate-x-1/2 size-[600px] rounded-full bg-brand/10 dark:bg-brand/15 blur-[120px]" />
      <header className="relative z-10 w-full px-4 sm:px-8 py-4 flex items-center justify-center border-b border-border/30 bg-background/50 backdrop-blur-xs">
        <Link to="/login" className="flex items-center gap-2">
          <div className="size-6 rounded-lg bg-brand/10 dark:bg-brand/20 p-0.5 flex items-center justify-center ring-1 ring-brand/30">
            <AnimatedHerbieLogo mood="float" size="sm" interactive={false} />
          </div>
          <span className="font-bold text-sm tracking-tight">Herbie</span>
        </Link>
      </header>
      <main className="relative z-10 flex flex-1 items-center justify-center p-4 sm:p-6 my-6">
        <div className="w-full max-w-md rounded-3xl border border-border/80 bg-card/85 p-6 sm:p-8 shadow-xl backdrop-blur-md space-y-5">
          <div className="space-y-1 text-center">
            <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
            <p className="text-xs sm:text-sm text-muted-foreground">{subtitle}</p>
          </div>
          {children}
        </div>
      </main>
      <footer className="relative z-10 w-full py-4 text-center text-xs text-muted-foreground/60 border-t border-border/20">
        Herbie AI · MIT Licensed
      </footer>
    </div>
  );
}
