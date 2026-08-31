import { useAuth } from "@/stores/auth";

const BASE = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

let refreshing: Promise<boolean> | null = null;

export async function tryRefresh(): Promise<boolean> {
  refreshing ??= fetch(`${BASE}/api/auth/refresh`, {
    method: "POST",
    credentials: "include",
  }).then(async (res) => {
    if (!res.ok) return false;
    const data = (await res.json()) as { accessToken: string };
    const { user, setAuth } = useAuth.getState();
    if (user) setAuth(user, data.accessToken);
    return true;
  }).finally(() => { refreshing = null; });
  return refreshing;
}

export async function apiFetch<T>(path: string, opts: RequestInit & { json?: unknown } = {}): Promise<T> {
  const doFetch = () => {
    const headers = new Headers(opts.headers);
    if (opts.json !== undefined) headers.set("Content-Type", "application/json");
    const token = useAuth.getState().accessToken;
    if (token) headers.set("Authorization", `Bearer ${token}`);
    return fetch(`${BASE}${path}`, {
      ...opts,
      headers,
      body: opts.json !== undefined ? JSON.stringify(opts.json) : opts.body,
      credentials: "include",
    });
  };

  let res = await doFetch();
  if (res.status === 401 && (await tryRefresh())) {
    res = await doFetch();
  }
  if (!res.ok) {
    let code = "error", message = `request failed (${res.status})`;
    try {
      const body = (await res.json()) as { error?: { code: string; message: string } };
      if (body.error) { code = body.error.code; message = body.error.message; }
    } catch { /* non-JSON error body */ }
    if (res.status === 401) useAuth.getState().clear();
    throw new ApiError(res.status, code, message);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export { BASE };
