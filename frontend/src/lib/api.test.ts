import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { apiFetch, tryRefresh } from "./api";
import { useAuth } from "@/stores/auth";

const server = setupServer(
  http.post("*/api/auth/refresh", () => HttpResponse.json({ accessToken: "fresh" })),
  http.get("*/api/things", ({ request }) => {
    if (request.headers.get("Authorization") === "Bearer fresh") {
      return HttpResponse.json({ ok: true });
    }
    return HttpResponse.json({ error: { code: "unauthorized", message: "no" } }, { status: 401 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => {
  useAuth.setState({ user: { id: "u", email: "a@b.co" }, accessToken: "stale" });
});
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

describe("apiFetch", () => {
  it("refreshes once on 401 and retries with the new token", async () => {
    const res = await apiFetch<{ ok: boolean }>("/api/things");
    expect(res.ok).toBe(true);
    expect(useAuth.getState().accessToken).toBe("fresh");
  });
});

describe("tryRefresh", () => {
  it("restores user and token on fresh reload when returned by server", async () => {
    server.use(
      http.post("*/api/auth/refresh", () =>
        HttpResponse.json({
          accessToken: "restored-token",
          user: { id: "u-reloaded", email: "reloaded@test.com" },
        }),
      ),
    );
    useAuth.getState().clear();

    const ok = await tryRefresh();
    expect(ok).toBe(true);
    expect(useAuth.getState()).toEqual({
      accessToken: "restored-token",
      user: { id: "u-reloaded", email: "reloaded@test.com" },
      setAuth: expect.any(Function),
      clear: expect.any(Function),
    });
  });

  it("returns false and leaves auth cleared when refresh rejects", async () => {
    server.use(
      http.post("*/api/auth/refresh", () =>
        HttpResponse.json({ error: { code: "unauthorized", message: "no cookie" } }, { status: 401 }),
      ),
    );
    useAuth.getState().clear();

    const ok = await tryRefresh();
    expect(ok).toBe(false);
    expect(useAuth.getState().accessToken).toBeNull();
    expect(useAuth.getState().user).toBeNull();
  });
});
