import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { ForgotPasswordPage } from "@/pages/ForgotPasswordPage";
import { ResetPasswordPage } from "@/pages/ResetPasswordPage";
import { VerifyEmailPage } from "@/pages/VerifyEmailPage";

const server = setupServer(
  http.post("*/api/auth/forgot-password", () => HttpResponse.json({ accepted: true }, { status: 202 })),
  http.post("*/api/auth/reset-password", () => new HttpResponse(null, { status: 204 })),
  http.post("*/api/auth/verify-email", () => new HttpResponse(null, { status: 204 })),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

function renderApp(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/forgot-password" element={<ForgotPasswordPage />} />
        <Route path="/reset-password" element={<ResetPasswordPage />} />
        <Route path="/verify-email" element={<VerifyEmailPage />} />
        <Route path="/login" element={<div>login page</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("auth pages", () => {
  it("requests a password reset without revealing account existence", async () => {
    renderApp("/forgot-password");
    await userEvent.type(screen.getByLabelText("Email"), "a@b.co");
    await userEvent.click(screen.getByRole("button", { name: "Send reset link" }));
    expect(await screen.findByText(/If that email exists/)).toBeInTheDocument();
  });

  it("resets the password and returns to sign in", async () => {
    renderApp("/reset-password#token=abc");
    await userEvent.type(screen.getByLabelText("New password"), "longenoughpass");
    await userEvent.type(screen.getByLabelText("Confirm password"), "longenoughpass");
    await userEvent.click(screen.getByRole("button", { name: "Update password" }));
    expect(await screen.findByText("login page")).toBeInTheDocument();
  });

  it("shows a link to request a new one when the token is missing", () => {
    renderApp("/reset-password");
    expect(screen.getByRole("link", { name: "Request a new link" })).toBeInTheDocument();
  });

  it("verifies an email token", async () => {
    renderApp("/verify-email#token=abc");
    expect(await screen.findByRole("link", { name: "Continue to sign in" })).toBeInTheDocument();
  });
});
