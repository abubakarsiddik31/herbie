import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeEach, describe, expect, it } from "vitest";
import { apiFetch } from "./api";
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

beforeEach(() => {
  server.listen({ onUnhandledRequest: "error" });
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
