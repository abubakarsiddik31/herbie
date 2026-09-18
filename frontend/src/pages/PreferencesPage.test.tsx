import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { PreferencesPage } from "./PreferencesPage";

let saved: string[] = [];

const server = setupServer(
  http.get("*/api/profile", () => HttpResponse.json({ defaultInstructions: "Be concise." })),
  http.patch("*/api/profile", async ({ request }) => {
    const body = (await request.json()) as { defaultInstructions: string };
    saved.push(body.defaultInstructions);
    return new HttpResponse(null, { status: 204 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
  saved = [];
});
afterAll(() => server.close());

function renderPreferences() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/preferences"]}>
        <Routes>
          <Route path="/preferences" element={<PreferencesPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("PreferencesPage", () => {
  it("loads, edits, and saves custom instructions", async () => {
    renderPreferences();
    const box = await screen.findByLabelText("Custom instructions");
    expect(box).toHaveValue("Be concise.");
    expect(screen.getByText("11 / 4000")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

    await userEvent.clear(box);
    await userEvent.type(box, "Be terse.");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(saved).toEqual(["Be terse."]);
  });
});
