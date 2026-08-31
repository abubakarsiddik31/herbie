import { act } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter, Route, Routes } from "react-router";
import { toast } from "sonner";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { RequireAuth } from "@/components/RequireAuth";
import { LoginPage } from "@/pages/LoginPage";
import { RegisterPage } from "@/pages/RegisterPage";
import { useAuth } from "@/stores/auth";

const server = setupServer(
  http.post("*/api/auth/login", () =>
    HttpResponse.json({ accessToken: "tok", user: { id: "u1", email: "a@b.co" } }),
  ),
  http.post("*/api/auth/register", () =>
    HttpResponse.json({ accessToken: "reg-tok", user: { id: "u2", email: "c@d.co" } }),
  ),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  act(() => useAuth.getState().clear());
  server.resetHandlers();
});
afterAll(() => server.close());

function renderRoutes(initial: string) {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="/" element={<div>guarded home</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AuthForm", () => {
  it("logs in, stores the token in the auth store, and navigates home", async () => {
    renderRoutes("/login");
    await userEvent.type(screen.getByLabelText("Email"), "a@b.co");
    await userEvent.type(screen.getByLabelText("Password"), "hunter2hunter2");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByText("guarded home")).toBeInTheDocument();
    expect(useAuth.getState()).toMatchObject({
      accessToken: "tok",
      user: { id: "u1", email: "a@b.co" },
    });
  });

  it("registers with a 10+ char password and navigates home", async () => {
    renderRoutes("/register");
    await userEvent.type(screen.getByLabelText("Email"), "c@d.co");
    await userEvent.type(screen.getByLabelText("Password"), "longenoughpass");
    await userEvent.click(screen.getByRole("button", { name: "Sign up" }));

    expect(await screen.findByText("guarded home")).toBeInTheDocument();
    expect(useAuth.getState()).toMatchObject({
      accessToken: "reg-tok",
      user: { id: "u2", email: "c@d.co" },
    });
  });

  it("surfaces API errors via toast and stays on the page", async () => {
    server.use(
      http.post("*/api/auth/login", () =>
        HttpResponse.json({ error: { code: "invalid_credentials", message: "bad credentials" } }, { status: 400 }),
      ),
    );
    const errorSpy = vi.spyOn(toast, "error").mockReturnValue("");
    renderRoutes("/login");
    await userEvent.type(screen.getByLabelText("Email"), "a@b.co");
    await userEvent.type(screen.getByLabelText("Password"), "wrongpassword");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));

    await vi.waitFor(() => expect(errorSpy).toHaveBeenCalledWith("bad credentials"));
    expect(screen.queryByText("guarded home")).not.toBeInTheDocument();
    expect(useAuth.getState().accessToken).toBeNull();
    errorSpy.mockRestore();
  });

  it("RequireAuth redirects to /login when unauthenticated", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route element={<RequireAuth />}>
            <Route path="/" element={<div>guarded home</div>} />
          </Route>
          <Route path="/login" element={<div>login page</div>} />
        </Routes>
      </MemoryRouter>,
    );
    expect(screen.getByText("login page")).toBeInTheDocument();
    expect(screen.queryByText("guarded home")).not.toBeInTheDocument();
  });

  it("RequireAuth renders guarded children when authenticated", () => {
    act(() => useAuth.setState({ user: { id: "u1", email: "a@b.co" }, accessToken: "tok" }));
    render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route element={<RequireAuth />}>
            <Route path="/" element={<div>guarded home</div>} />
          </Route>
          <Route path="/login" element={<div>login page</div>} />
        </Routes>
      </MemoryRouter>,
    );
    expect(screen.getByText("guarded home")).toBeInTheDocument();
  });
});
