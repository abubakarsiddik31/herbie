import { act } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { OAuthCallbackPage } from "@/pages/OAuthCallbackPage";
import { useAuth } from "@/stores/auth";

const server = setupServer(
  http.post("*/api/auth/oauth/consume", () =>
    HttpResponse.json({ accessToken: "social-tok", user: { id: "u9", email: "s@x.co" } }),
  ),
  http.post("*/api/auth/refresh", () =>
    HttpResponse.json(
      { error: { code: "unauthorized", message: "refresh rejected" } },
      { status: 401 },
    ),
  ),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  act(() => useAuth.getState().clear());
  server.resetHandlers();
});
afterAll(() => server.close());

function renderCallback(entry: string) {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path="/auth/callback" element={<OAuthCallbackPage />} />
        <Route path="/login" element={<div>login page</div>} />
        <Route path="/" element={<div>guarded home</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("OAuthCallbackPage", () => {
  it("exchanges the code, stores the session, and navigates home", async () => {
    renderCallback("/auth/callback?code=one-time");
    expect(await screen.findByText("guarded home")).toBeInTheDocument();
    expect(useAuth.getState()).toMatchObject({
      accessToken: "social-tok",
      user: { id: "u9", email: "s@x.co" },
    });
  });

  it("shows a failure with a way back when the provider reports an error", async () => {
    renderCallback("/auth/callback?error=denied");
    expect(await screen.findByText("Sign-in failed")).toBeInTheDocument();
    expect(screen.getByText(/cancelled/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to sign in" })).toHaveAttribute("href", "/login");
    expect(useAuth.getState().accessToken).toBeNull();
  });

  it("shows a failure when the code exchange is rejected", async () => {
    server.use(
      http.post("*/api/auth/oauth/consume", () =>
        HttpResponse.json(
          { error: { code: "invalid_code", message: "invalid or expired oauth code" } },
          { status: 401 },
        ),
      ),
    );
    const clearSpy = vi.spyOn(useAuth.getState(), "clear");
    renderCallback("/auth/callback?code=stale");
    expect(await screen.findByText("Sign-in failed")).toBeInTheDocument();
    expect(screen.getByText(/expired or was already used/)).toBeInTheDocument();
    expect(clearSpy).toHaveBeenCalled();
    clearSpy.mockRestore();
  });
});
