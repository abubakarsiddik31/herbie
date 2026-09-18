import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router";
import { toast } from "sonner";
import { ApiError, BASE, apiFetch } from "@/lib/api";
import type { User } from "@/lib/types";
import { useAuth } from "@/stores/auth";
import { useOAuthProviders } from "./useOAuthProviders";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const copy = {
  login: {
    title: "Welcome back",
    subtitle: "Sign in to continue to Golem",
    cta: "Sign in",
    endpoint: "/api/auth/login",
    footer: "No account?",
    link: "/register",
    linkLabel: "Create one",
  },
  register: {
    title: "Create your account",
    subtitle: "Get started with Golem",
    cta: "Sign up",
    endpoint: "/api/auth/register",
    footer: "Already have an account?",
    link: "/login",
    linkLabel: "Sign in",
  },
} as const;

export function AuthForm({ mode }: { mode: keyof typeof copy }) {
  const t = copy[mode];
  const navigate = useNavigate();
  const providers = useOAuthProviders();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
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
    <div className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="text-2xl">{t.title}</CardTitle>
          <CardDescription>{t.subtitle}</CardDescription>
        </CardHeader>
        <CardContent>
          {providers !== null && providers.length > 0 && (
            <>
              <div className="grid gap-2">
                {providers.map((p) => (
                  <Button key={p.id} variant="outline" asChild>
                    <a href={`${BASE}/api/auth/oauth/${p.id}`}>Continue with {p.name}</a>
                  </Button>
                ))}
              </div>
              <div className="my-4 flex items-center gap-2 text-xs text-muted-foreground">
                <span className="h-px flex-1 bg-border" />
                or
                <span className="h-px flex-1 bg-border" />
              </div>
            </>
          )}
          <form onSubmit={onSubmit} className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                autoComplete="email"
                placeholder="you@example.com"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                autoComplete={mode === "login" ? "current-password" : "new-password"}
                placeholder={mode === "register" ? "At least 10 characters" : "Your password"}
                required
                minLength={mode === "register" ? 10 : undefined}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            <Button type="submit" disabled={pending}>
              {pending ? "Please wait…" : t.cta}
            </Button>
          </form>
        </CardContent>
        <CardFooter className="justify-center gap-1 text-sm text-muted-foreground">
          {t.footer}
          <Link to={t.link} className="underline underline-offset-4 hover:text-foreground">
            {t.linkLabel}
          </Link>
        </CardFooter>
      </Card>
    </div>
  );
}
