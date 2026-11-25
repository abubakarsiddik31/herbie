import { useEffect, useRef, useState } from "react";
import { Link, useLocation, useSearchParams } from "react-router";
import { ApiError, apiFetch } from "@/lib/api";
import { AuthPageShell } from "@/features/auth/AuthPageShell";
import { Button } from "@/components/ui/button";

type State = "verifying" | "done" | "error";

export function VerifyEmailPage() {
  const [params] = useSearchParams();
  const location = useLocation();
  const [token] = useState(
    () =>
      new URLSearchParams(location.hash.replace(/^#/, "")).get("token") ??
      params.get("token") ??
      "",
  );
  const ran = useRef(false);
  const [state, setState] = useState<State>(token ? "verifying" : "error");
  const [message, setMessage] = useState(token ? "" : "This link is missing its token.");

  useEffect(() => {
    if (!token || ran.current) return;
    ran.current = true;
    window.history.replaceState(null, "", window.location.pathname);
    let cancelled = false;
    apiFetch<void>("/api/auth/verify-email", { method: "POST", json: { token } })
      .then(() => {
        if (!cancelled) setState("done");
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setMessage(err instanceof ApiError ? err.message : "This link is invalid or has expired.");
        setState("error");
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  return (
    <AuthPageShell
      title="Verify email"
      subtitle={
        state === "verifying"
          ? "Checking your link…"
          : state === "done"
            ? "Your email is verified."
            : "We could not verify this link."
      }
    >
      {state === "verifying" && (
        <div className="flex justify-center py-2">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        </div>
      )}
      {state === "done" && (
        <Button variant="brand" className="w-full" asChild>
          <Link to="/login">Continue to sign in</Link>
        </Button>
      )}
      {state === "error" && (
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground text-center">{message}</p>
          <Button variant="outline" className="w-full" asChild>
            <Link to="/login">Back to sign in</Link>
          </Button>
        </div>
      )}
    </AuthPageShell>
  );
}
