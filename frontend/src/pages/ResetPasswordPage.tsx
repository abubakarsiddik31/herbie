import { useEffect, useState, type FormEvent } from "react";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { ApiError, apiFetch } from "@/lib/api";
import { AuthPageShell } from "@/features/auth/AuthPageShell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function ResetPasswordPage() {
  const [params] = useSearchParams();
  const location = useLocation();
  const [token] = useState(
    () =>
      new URLSearchParams(location.hash.replace(/^#/, "")).get("token") ??
      params.get("token") ??
      "",
  );
  const navigate = useNavigate();
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [pending, setPending] = useState(false);

  useEffect(() => {
    if (location.hash || params.get("token")) {
      window.history.replaceState(null, "", window.location.pathname);
    }
  }, [location.hash, params]);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (password !== confirm) {
      toast.error("Passwords do not match");
      return;
    }
    setPending(true);
    try {
      await apiFetch<void>("/api/auth/reset-password", {
        method: "POST",
        json: { token, password },
      });
      toast.success("Password updated. Sign in with your new password.");
      navigate("/login", { replace: true });
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not reset password");
    } finally {
      setPending(false);
    }
  }

  if (!token) {
    return (
      <AuthPageShell title="Reset password" subtitle="This link is missing its token.">
        <Button variant="outline" className="w-full" asChild>
          <Link to="/forgot-password">Request a new link</Link>
        </Button>
      </AuthPageShell>
    );
  }

  return (
    <AuthPageShell title="Choose a new password" subtitle="Your other sessions will be signed out.">
      <form onSubmit={onSubmit} className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="password" className="text-xs font-medium">New password</Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            minLength={10}
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="h-10 text-sm bg-background/60"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="confirm" className="text-xs font-medium">Confirm password</Label>
          <Input
            id="confirm"
            type="password"
            autoComplete="new-password"
            minLength={10}
            required
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            className="h-10 text-sm bg-background/60"
          />
        </div>
        <Button type="submit" variant="brand" className="w-full h-10" disabled={pending}>
          {pending ? "Saving…" : "Update password"}
        </Button>
      </form>
    </AuthPageShell>
  );
}
