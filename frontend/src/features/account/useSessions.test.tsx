import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { useSessions, useRevokeSession, useRevokeAllSessions } from "./useSessions";

const now = new Date().toISOString();
const sessions = [
  { id: "s1", userAgent: "Chrome", ip: "1.2.3.4", createdAt: now, lastUsedAt: now, current: true },
  { id: "s2", userAgent: "Firefox", ip: "5.6.7.8", createdAt: now, lastUsedAt: now, current: false },
];

let revoked: string[] = [];

const server = setupServer(
  http.get("*/api/account/sessions", () => HttpResponse.json({ sessions })),
  http.delete("*/api/account/sessions/:id", ({ params }) => {
    revoked.push(String(params.id));
    return new HttpResponse(null, { status: 204 });
  }),
  http.delete("*/api/account/sessions", () => {
    revoked.push("all");
    return new HttpResponse(null, { status: 204 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  revoked = [];
  server.resetHandlers();
});
afterAll(() => server.close());

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("sessions hooks", () => {
  it("lists sessions", async () => {
    const { result } = renderHook(() => useSessions(), { wrapper });
    await waitFor(() => expect(result.current.data).toHaveLength(2));
    expect(result.current.data?.[0]).toMatchObject({ id: "s1", current: true });
  });

  it("revokes one session and all sessions", async () => {
    const one = renderHook(() => useRevokeSession(), { wrapper });
    await act(async () => {
      await one.result.current.mutateAsync("s2");
    });
    const all = renderHook(() => useRevokeAllSessions(), { wrapper });
    await act(async () => {
      await all.result.current.mutateAsync();
    });
    expect(revoked).toEqual(["s2", "all"]);
  });
});
