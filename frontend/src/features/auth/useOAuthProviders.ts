import { useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import type { OAuthProvider } from "@/lib/types";

// Providers configured on the server. Null while loading, empty when the
// server offers password auth only.
export function useOAuthProviders() {
  const [providers, setProviders] = useState<OAuthProvider[] | null>(null);

  useEffect(() => {
    let live = true;
    apiFetch<{ providers: OAuthProvider[] }>("/api/auth/providers")
      .then((res) => {
        if (live) setProviders(res.providers);
      })
      .catch(() => {
        if (live) setProviders([]);
      });
    return () => {
      live = false;
    };
  }, []);

  return providers;
}
