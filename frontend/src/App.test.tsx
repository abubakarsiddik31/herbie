import { cleanup, render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { act } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import App from "./App";
import { useAuth } from "./stores/auth";

const server = setupServer(
  http.post("*/api/auth/refresh", () =>
    HttpResponse.json({
      accessToken: "refreshed-tok",
      user: { id: "u-refreshed", email: "refreshed@example.com" },
    }),
  ),
  http.get("*/api/auth/providers", () => HttpResponse.json({ providers: [] })),
  http.get("*/api/conversations", () => HttpResponse.json([])),
  http.get("*/api/profile", () => HttpResponse.json({ instructions: "" })),
  http.get("*/api/mcp/servers", () => HttpResponse.json({ servers: [] })),
  http.get("*/api/mcp/catalog", () => HttpResponse.json({ catalog: [] })),
  http.get("*/api/tools/oauth/status", () => HttpResponse.json({ connected: [] })),
  http.get("*/api/tool-oauth/providers", () => HttpResponse.json([])),
  http.get("*/api/tools", () => HttpResponse.json([])),
  http.get("*/api/projects", () => HttpResponse.json({ projects: [] })),
  http.get("*/api/workflows", () => HttpResponse.json({ workflows: [] })),
  http.get("*/api/documents", () => HttpResponse.json({ documents: [] })),
  http.get("*/api/models", () =>
    HttpResponse.json({
      default: "gpt-4o",
      models: [{ id: "gpt-4o", label: "GPT-4o" }],
    }),
  ),
  http.get("*/api/memories", () => HttpResponse.json([])),
);

beforeAll(() => server.listen({ onUnhandledRequest: "warn" }));
afterEach(() => {
  cleanup();
  act(() => useAuth.getState().clear());
  server.resetHandlers();
  window.history.pushState({}, "", "/");
});
afterAll(() => server.close());

describe("App silent refresh on reload", () => {
  it("restores user session on page refresh when refresh cookie is valid", async () => {
    window.history.pushState({}, "", "/chat");
    render(<App />);

    // Initially loading
    expect(useAuth.getState().accessToken).toBeNull();

    // After refresh completes, session is restored and user stays in protected app
    await screen.findByRole("button", { name: /new chat/i });
    expect(useAuth.getState()).toMatchObject({
      accessToken: "refreshed-tok",
      user: { id: "u-refreshed", email: "refreshed@example.com" },
    });
  });

  it("redirects to /login when refresh rejects", async () => {
    server.use(
      http.post("*/api/auth/refresh", () =>
        HttpResponse.json({ error: { code: "unauthorized", message: "unauthorized" } }, { status: 401 }),
      ),
    );
    window.history.pushState({}, "", "/chat");
    render(<App />);

    // Should redirect to login page
    expect(await screen.findByRole("button", { name: "Sign in" })).toBeInTheDocument();
    expect(useAuth.getState().accessToken).toBeNull();
  });

  it("redirects from /login to /chat when already authenticated", async () => {
    window.history.pushState({}, "", "/login");
    render(<App />);

    // Refresh succeeds, so user gets redirected to chat
    await screen.findByRole("button", { name: /new chat/i });
    expect(screen.queryByRole("button", { name: "Sign in" })).not.toBeInTheDocument();
  });
});
