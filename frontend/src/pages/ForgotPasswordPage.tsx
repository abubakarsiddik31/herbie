import { useState, type FormEvent } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { ApiError, apiFetch } from "@/lib/api";
import { AuthPageShell } from "@/features/auth/AuthPageShell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [pending, setPending] = useState(false);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setPending(true);
    try {
      await apiFetch<void>("/api/auth/forgot-password", { method: "POST", json: { email } });
      setSent(true);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Something went wrong");
    } finally {
      setPending(false);
    }
  }

  return (
    <AuthPageShell
      title="Forgot password"
      subtitle="We will email you a link to choose a new password."
    >
      {sent ? (
        <div className="space-y-4 text-center">
          <p className="text-sm text-muted-foreground">
            If that email exists, a reset link is on its way. It expires in 30 minutes.
          </p>
          <Button variant="outline" className="w-full" asChild>
            <Link to="/login">Back to sign in</Link>
          </Button>
        </div>
      ) : (
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="email" className="text-xs font-medium">Email</Label>
            <Input
              id="email"
              type="email"
              autoComplete="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="h-10 text-sm bg-background/60"
            />
          </div>
          <Button type="submit" variant="brand" className="w-full h-10" disabled={pending}>
            {pending ? "Sending…" : "Send reset link"}
          </Button>
          <div className="text-center text-xs text-muted-foreground">
            <Link to="/login" className="font-medium text-brand hover:underline underline-offset-4">
              Back to sign in
            </Link>
          </div>
        </form>
      )}
    </AuthPageShell>
  );
}
