import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { PreferencesPage } from "./PreferencesPage";

let saved: string[] = [];
let mockMemories: Array<{ id: string; userId: string; content: string; createdAt: string; updatedAt: string }> = [];

const server = setupServer(
  http.get("*/api/profile", () => HttpResponse.json({ defaultInstructions: "Be concise." })),
  http.patch("*/api/profile", async ({ request }) => {
    const body = (await request.json()) as { defaultInstructions: string };
    saved.push(body.defaultInstructions);
    return new HttpResponse(null, { status: 204 });
  }),
  http.get("*/api/memories", () => HttpResponse.json(mockMemories)),
  http.post("*/api/memories", async ({ request }) => {
    const body = (await request.json()) as { content: string };
    const created = {
      id: `mem-${mockMemories.length + 1}`,
      userId: "u-1",
      content: body.content,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    mockMemories.unshift(created);
    return HttpResponse.json(created, { status: 201 });
  }),
  http.delete("*/api/memories/:id", ({ params }) => {
    mockMemories = mockMemories.filter((m) => m.id !== params.id);
    return new HttpResponse(null, { status: 204 });
  }),
  http.delete("*/api/memories", () => {
    mockMemories = [];
    return new HttpResponse(null, { status: 204 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
  saved = [];
  mockMemories = [];
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

  it("adds, lists, and deletes memories", async () => {
    mockMemories = [
      {
        id: "mem-1",
        userId: "u-1",
        content: "Prefers TypeScript",
        createdAt: "2026-09-19T00:00:00Z",
        updatedAt: "2026-09-19T00:00:00Z",
      },
    ];

    renderPreferences();
    expect(await screen.findByText("Prefers TypeScript")).toBeInTheDocument();

    // Add new memory
    const input = screen.getByLabelText("New memory");
    await userEvent.type(input, "Lives in Paris");
    await userEvent.click(screen.getByRole("button", { name: /add/i }));

    expect(await screen.findByText("Lives in Paris")).toBeInTheDocument();

    // Delete a memory
    const deleteBtn = screen.getByLabelText("Delete memory: Prefers TypeScript");
    await userEvent.click(deleteBtn);
    expect(screen.queryByText("Prefers TypeScript")).not.toBeInTheDocument();
  });

  it("clears all memories when confirmed", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    mockMemories = [
      {
        id: "mem-1",
        userId: "u-1",
        content: "Fact 1",
        createdAt: "2026-09-19T00:00:00Z",
        updatedAt: "2026-09-19T00:00:00Z",
      },
    ];

    renderPreferences();
    expect(await screen.findByText("Fact 1")).toBeInTheDocument();

    const clearBtn = screen.getByRole("button", { name: "Clear all" });
    await userEvent.click(clearBtn);

    expect(screen.queryByText("Fact 1")).not.toBeInTheDocument();
    expect(await screen.findByText(/no memories saved yet/i)).toBeInTheDocument();
  });
});
