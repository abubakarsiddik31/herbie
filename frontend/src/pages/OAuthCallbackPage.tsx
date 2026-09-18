import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { apiFetch } from "@/lib/api";
import type { User } from "@/lib/types";
import { useAuth } from "@/stores/auth";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

const failures: Record<string, string> = {
  denied: "The sign-in was cancelled before it completed.",
  invalid_state: "This sign-in attempt expired. Please try again.",
  unverified_email: "Your provider account shared no verified email address.",
  account_taken: "That provider account is already linked to a different user.",
  provider_error: "The provider rejected the sign-in. Please try again.",
};

export function OAuthCallbackPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const [failure, setFailure] = useState<string | null>(null);

  useEffect(() => {
    const error = params.get("error");
    const code = params.get("code");
    if (error || !code) {
      setFailure(failures[error ?? ""] ?? "Something went wrong signing you in.");
      return;
    }
    apiFetch<{ accessToken: string; user: User }>("/api/auth/oauth/consume", {
      method: "POST",
      json: { code },
    })
      .then((res) => {
        useAuth.getState().setAuth(res.user, res.accessToken);
        navigate("/", { replace: true });
      })
      .catch(() => setFailure("This sign-in link expired or was already used. Please try again."));
  }, [params, navigate]);

  return (
    <div className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="text-2xl">{failure ? "Sign-in failed" : "Signing you in…"}</CardTitle>
          {failure && <CardDescription>{failure}</CardDescription>}
        </CardHeader>
        {failure && (
          <CardContent>
            <Button asChild className="w-full">
              <Link to="/login">Back to sign in</Link>
            </Button>
          </CardContent>
        )}
      </Card>
    </div>
  );
}
