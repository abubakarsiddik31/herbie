import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router";
import { toast } from "sonner";
import { ArrowLeft, Eye, EyeOff, Lock, Mail, ShieldCheck } from "lucide-react";
import { ApiError, BASE, apiFetch } from "@/lib/api";
import type { User } from "@/lib/types";
import { useAuth } from "@/stores/auth";
import { useOAuthProviders } from "./useOAuthProviders";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ThemeToggle } from "@/components/ThemeToggle";
import { AnimatedHerbieLogo } from "@/components/AnimatedHerbieLogo";

const copy = {
  login: {
    title: "Welcome back",
    subtitle: "Sign in to access your Herbie workspace",
    cta: "Sign in",
    endpoint: "/api/auth/login",
    footer: "No account?",
    link: "/register",
    linkLabel: "Create one",
    greeting: "“Welcome back! Ready to run some pipelines?”",
    mood: "waving" as const,
  },
  register: {
    title: "Create your account",
    subtitle: "Start your sovereign, local-first AI workspace",
    cta: "Sign up",
    endpoint: "/api/auth/register",
    footer: "Already have an account?",
    link: "/login",
    linkLabel: "Sign in",
    greeting: "“Hello! Let's set up your private workspace!”",
    mood: "jumping" as const,
  },
} as const;

export function AuthForm({ mode }: { mode: keyof typeof copy }) {
  const t = copy[mode];
  const navigate = useNavigate();
  const providers = useOAuthProviders();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [pending, setPending] = useState(false);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setPending(true);
    try {
      const res = await apiFetch<{ accessToken: string; user: User }>(t.endpoint, {
        method: "POST",
        json: { email, password },
      });
      useAuth.getState().setAuth(res.user, res.accessToken);
      navigate("/", { replace: true });
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Something went wrong");
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="relative min-h-screen flex flex-col justify-between bg-background text-foreground selection:bg-brand/20 selection:text-brand antialiased overflow-x-hidden">
      {/* Background Ambient Glow */}
      <div className="pointer-events-none absolute -top-40 left-1/2 -translate-x-1/2 size-[600px] rounded-full bg-brand/10 dark:bg-brand/15 blur-[120px]" />
      <div className="pointer-events-none absolute bottom-0 right-0 size-[400px] rounded-full bg-emerald-500/5 blur-[100px]" />

      {/* Top Navbar */}
      <header className="relative z-10 w-full px-4 sm:px-8 py-4 flex items-center justify-between border-b border-border/30 bg-background/50 backdrop-blur-xs">
        <Link
          to="/"
          className="inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors group"
        >
          <ArrowLeft className="size-3.5 group-hover:-translate-x-0.5 transition-transform" />
          <span>Back to Overview</span>
        </Link>

        <div className="flex items-center gap-3">
          <Link to="/" className="flex items-center gap-2">
            <div className="size-6 rounded-lg bg-brand/10 dark:bg-brand/20 p-0.5 flex items-center justify-center ring-1 ring-brand/30">
              <AnimatedHerbieLogo mood="float" size="sm" interactive={false} />
            </div>
            <span className="font-bold text-sm tracking-tight">Herbie</span>
          </Link>
          <div className="h-4 w-px bg-border/60" />
          <ThemeToggle />
        </div>
      </header>

      {/* Main Center Stage */}
      <main className="relative z-10 flex flex-1 items-center justify-center p-4 sm:p-6 my-6">
        <div className="w-full max-w-md">
          {/* Card Container */}
          <div className="rounded-3xl border border-border/80 bg-card/85 p-6 sm:p-8 shadow-xl backdrop-blur-md space-y-6">
            {/* Mascot Greeting Header */}
            <div className="flex flex-col items-center text-center space-y-2">
              <div className="pb-1">
                <AnimatedHerbieLogo
                  mood={t.mood}
                  size="md"
                  interactive={true}
                  bubbleText={t.greeting}
                />
              </div>
              <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">{t.title}</h1>
              <p className="text-xs sm:text-sm text-muted-foreground max-w-xs">{t.subtitle}</p>
            </div>

            {/* OAuth Providers */}
            {providers !== null && providers.length > 0 && (
              <div className="space-y-3">
                <div className="grid gap-2">
                  {providers.map((p) => (
                    <Button key={p.id} variant="outline" className="w-full h-10 text-xs font-medium gap-2" asChild>
                      <a href={`${BASE}/api/auth/oauth/${p.id}`}>Continue with {p.name}</a>
                    </Button>
                  ))}
                </div>
                <div className="flex items-center gap-3 text-[11px] text-muted-foreground uppercase tracking-wider">
                  <span className="h-px flex-1 bg-border/80" />
                  <span>or email</span>
                  <span className="h-px flex-1 bg-border/80" />
                </div>
              </div>
            )}

            {/* Auth Form */}
            <form onSubmit={onSubmit} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="email" className="text-xs font-medium text-foreground flex items-center gap-1.5">
                  <Mail className="size-3.5 text-muted-foreground" />
                  <span>Email</span>
                </Label>
                <Input
                  id="email"
                  type="email"
                  autoComplete="email"
                  placeholder="you@example.com"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="h-10 text-sm bg-background/60"
                />
              </div>

              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <Label htmlFor="password" className="text-xs font-medium text-foreground flex items-center gap-1.5">
                    <Lock className="size-3.5 text-muted-foreground" />
                    <span>Password</span>
                  </Label>
                  {mode === "register" && (
                    <span className="text-[11px] text-muted-foreground">Min. 10 chars</span>
                  )}
                </div>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? "text" : "password"}
                    autoComplete={mode === "login" ? "current-password" : "new-password"}
                    placeholder={mode === "register" ? "At least 10 characters" : "Your password"}
                    required
                    minLength={mode === "register" ? 10 : undefined}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="h-10 text-sm pr-10 bg-background/60"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword((prev) => !prev)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                    aria-label={showPassword ? "Hide password" : "Show password"}
                  >
                    {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </button>
                </div>
              </div>

              <Button
                type="submit"
                variant="brand"
                disabled={pending}
                className="w-full h-10 text-sm font-medium shadow-xs"
              >
                {pending ? "Please wait…" : t.cta}
              </Button>
            </form>

            {/* Footer switch */}
            <div className="pt-2 text-center text-xs text-muted-foreground">
              <span>{t.footer} </span>
              <Link to={t.link} className="font-medium text-brand hover:underline underline-offset-4">
                {t.linkLabel}
              </Link>
            </div>
          </div>

          {/* Privacy & Security Trust Footer */}
          <div className="mt-4 flex items-center justify-center gap-3 text-[11px] text-muted-foreground/80">
            <span className="inline-flex items-center gap-1">
              <ShieldCheck className="size-3 text-emerald-500" />
              <span>Local SQLite</span>
            </span>
            <span>·</span>
            <span>Zero Telemetry</span>
            <span>·</span>
            <span>Direct Model Pricing</span>
          </div>
        </div>
      </main>

      {/* Page Footer */}
      <footer className="relative z-10 w-full py-4 text-center text-xs text-muted-foreground/60 border-t border-border/20">
        <span>Herbie AI · Inspired by Fantastic Four · MIT Licensed</span>
      </footer>
    </div>
  );
}
